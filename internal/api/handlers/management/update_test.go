package management

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestInferAutoUpdateChannel(t *testing.T) {
	tests := []struct {
		name    string
		version string
		env     string
		want    string
	}{
		{name: "explicit dev version", version: "dev-a35756e", want: "dev"},
		{name: "explicit main version", version: "main-a35756e", want: "main"},
		{name: "release tag defaults main", version: "v1.2.3", want: "main"},
		{name: "environment overrides version", version: "main-a35756e", env: "dev", want: "dev"},
		{name: "unknown environment ignored", version: "main-a35756e", env: "staging", want: "main"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferAutoUpdateChannel(tt.version, tt.env); got != tt.want {
				t.Fatalf("inferAutoUpdateChannel(%q, %q) = %q, want %q", tt.version, tt.env, got, tt.want)
			}
		})
	}
}

func TestAutoUpdateAvailableFromCommit(t *testing.T) {
	tests := []struct {
		name          string
		currentCommit string
		latestCommit  string
		want          bool
	}{
		{name: "same full commit", currentCommit: "abcdef123456", latestCommit: "abcdef123456", want: false},
		{name: "current short commit matches latest", currentCommit: "abcdef1", latestCommit: "abcdef123456", want: false},
		{name: "different commit", currentCommit: "1111111", latestCommit: "abcdef123456", want: true},
		{name: "missing latest commit", currentCommit: "1111111", latestCommit: "", want: false},
		{name: "missing current commit", currentCommit: "", latestCommit: "abcdef123456", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := autoUpdateAvailableFromCommit(tt.currentCommit, tt.latestCommit); got != tt.want {
				t.Fatalf("autoUpdateAvailableFromCommit(%q, %q) = %v, want %v", tt.currentCommit, tt.latestCommit, got, tt.want)
			}
		})
	}
}

func TestDockerTagForChannel(t *testing.T) {
	if got := dockerTagForChannel("dev", "a35756e"); got != "dev" {
		t.Fatalf("dockerTagForChannel(dev) = %q, want dev", got)
	}
	if got := dockerTagForChannel("main", "a35756e"); got != "latest" {
		t.Fatalf("dockerTagForChannel(main) = %q, want latest", got)
	}
}

func TestAutoUpdateChannelEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.AutoUpdate.Channel = config.DefaultAutoUpdateChannel
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	handler := NewHandler(cfg, configPath, nil)

	router := gin.New()
	router.GET("/channel", handler.GetAutoUpdateChannel)
	router.PUT("/channel", handler.PutAutoUpdateChannel)

	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/channel", nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), `"channel":"main"`) {
		t.Fatalf("GET body = %s, want channel main", getRec.Body.String())
	}

	putRec := httptest.NewRecorder()
	putReq := httptest.NewRequest(http.MethodPut, "/channel", strings.NewReader(`{"value":"dev"}`))
	putReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body=%s", putRec.Code, putRec.Body.String())
	}
	if cfg.AutoUpdate.Channel != "dev" {
		t.Fatalf("AutoUpdate.Channel = %q, want dev", cfg.AutoUpdate.Channel)
	}
}
