package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	defaultListenAddr    = ":8320"
	defaultComposeFile   = "/workspace/docker-compose.yml"
	defaultEnvFile       = "/workspace/.env"
	defaultTargetService = "clirelay"
	updateCommandTimeout = 10 * time.Minute
)

type composeRunner func(ctx context.Context, composeFile string, envFile string, projectName string, service string) error

type updaterConfig struct {
	Addr           string
	Token          string
	ComposeFile    string
	EnvFile        string
	ProjectName    string
	DefaultService string
	Runner         composeRunner
}

type updaterServer struct {
	token          string
	composeFile    string
	envFile        string
	projectName    string
	defaultService string
	runner         composeRunner
	mu             sync.Mutex
	lastStatus     string
	lastError      string
}

type updateRequest struct {
	Service string `json:"service"`
	Image   string `json:"image"`
	Tag     string `json:"tag"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Channel string `json:"channel"`
}

func main() {
	cfg := updaterConfig{
		Addr:           envOrDefault("CLIRELAY_UPDATER_ADDR", defaultListenAddr),
		Token:          strings.TrimSpace(os.Getenv("CLIRELAY_UPDATER_TOKEN")),
		ComposeFile:    envOrDefault("CLIRELAY_COMPOSE_FILE", defaultComposeFile),
		EnvFile:        envOrDefault("CLIRELAY_ENV_FILE", defaultEnvFile),
		ProjectName:    strings.TrimSpace(os.Getenv("CLIRELAY_COMPOSE_PROJECT_NAME")),
		DefaultService: envOrDefault("CLIRELAY_TARGET_SERVICE", defaultTargetService),
		Runner:         runComposeUpdate,
	}
	server := newUpdaterServer(cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", server.handleHealth)
	mux.HandleFunc("/v1/update", server.handleUpdate)

	log.Printf("clirelay updater listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func newUpdaterServer(cfg updaterConfig) *updaterServer {
	runner := cfg.Runner
	if runner == nil {
		runner = runComposeUpdate
	}
	return &updaterServer{
		token:          strings.TrimSpace(cfg.Token),
		composeFile:    envOrDefaultValue(cfg.ComposeFile, defaultComposeFile),
		envFile:        envOrDefaultValue(cfg.EnvFile, defaultEnvFile),
		projectName:    strings.TrimSpace(cfg.ProjectName),
		defaultService: envOrDefaultValue(cfg.DefaultService, defaultTargetService),
		runner:         runner,
		lastStatus:     "idle",
	}
}

func (s *updaterServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	s.mu.Lock()
	payload := map[string]string{"status": s.lastStatus, "error": s.lastError}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, payload)
}

func (s *updaterServer) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req updateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	service := sanitizeServiceName(req.Service)
	if service == "" {
		service = s.defaultService
	}
	if service == "" {
		http.Error(w, "missing target service", http.StatusBadRequest)
		return
	}

	if err := persistRequestedImage(s.envFile, req.Image, req.Tag); err != nil {
		message := "failed to update env file: " + err.Error()
		log.Print(message)
		s.setStatus("failed", message)
		http.Error(w, message, http.StatusInternalServerError)
		return
	}

	s.setStatus("running", "")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updateCommandTimeout)
		defer cancel()
		if err := s.runner(ctx, s.composeFile, s.envFile, s.projectName, service); err != nil {
			log.Printf("compose update failed: %v", err)
			s.setStatus("failed", err.Error())
			return
		}
		s.setStatus("completed", "")
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted", "service": service})
}

func persistRequestedImage(envFile string, image string, tag string) error {
	imageRef := requestedImageRef(image, tag)
	if imageRef == "" || strings.TrimSpace(envFile) == "" {
		return nil
	}

	data, err := os.ReadFile(envFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	line := "CLI_PROXY_IMAGE=" + imageRef
	lines := splitEnvLines(string(data))
	replaced := false
	for i, existing := range lines {
		if strings.HasPrefix(existing, "CLI_PROXY_IMAGE=") {
			lines[i] = line
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines, line)
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(envFile, []byte(content), 0o600)
}

func requestedImageRef(image string, tag string) string {
	cleanImage := strings.TrimSpace(image)
	cleanTag := strings.TrimSpace(tag)
	if cleanImage == "" || cleanTag == "" {
		return ""
	}
	if !isSafeImagePart(cleanImage) || !isSafeImagePart(cleanTag) {
		return ""
	}
	return fmt.Sprintf("%s:%s", cleanImage, cleanTag)
}

func splitEnvLines(content string) []string {
	trimmed := strings.TrimRight(content, "\r\n")
	if trimmed == "" {
		return nil
	}
	raw := strings.Split(trimmed, "\n")
	lines := raw[:0]
	for _, line := range raw {
		lines = append(lines, strings.TrimRight(line, "\r"))
	}
	return lines
}

func isSafeImagePart(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r <= ' ' || r == '\'' || r == '"' || r == '\\' || r == '`' || r == '$' {
			return false
		}
	}
	return true
}

func (s *updaterServer) authorized(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		value = strings.TrimSpace(value[len("Bearer "):])
	}
	return value == s.token
}

func (s *updaterServer) setStatus(status string, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastStatus = status
	s.lastError = message
}

func runComposeUpdate(ctx context.Context, composeFile string, envFile string, projectName string, service string) error {
	if err := runDockerCompose(ctx, composeFile, envFile, projectName, "pull", service); err != nil {
		return err
	}
	return runDockerCompose(ctx, composeFile, envFile, projectName, "up", "-d", "--remove-orphans", service)
}

func runDockerCompose(ctx context.Context, composeFile string, envFile string, projectName string, args ...string) error {
	base := buildComposeArgs(composeFile, envFile, projectName, args...)
	cmd := exec.CommandContext(ctx, "docker", base...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(string(output)) + ": " + err.Error())
	}
	return nil
}

func buildComposeArgs(composeFile string, envFile string, projectName string, args ...string) []string {
	base := []string{"compose"}
	if strings.TrimSpace(projectName) != "" {
		base = append(base, "--project-name", projectName)
	}
	if strings.TrimSpace(envFile) != "" {
		base = append(base, "--env-file", envFile)
	}
	if strings.TrimSpace(composeFile) != "" {
		base = append(base, "-f", composeFile)
	}
	base = append(base, args...)
	return base
}

func sanitizeServiceName(service string) string {
	trimmed := strings.TrimSpace(service)
	if trimmed == "" {
		return ""
	}
	for _, r := range trimmed {
		if !(r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return ""
		}
	}
	return trimmed
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func envOrDefault(key string, fallback string) string {
	return envOrDefaultValue(os.Getenv(key), fallback)
}

func envOrDefaultValue(value string, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}
