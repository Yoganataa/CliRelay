package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestUpdaterRejectsInvalidBearerToken(t *testing.T) {
	server := newUpdaterServer(updaterConfig{
		Token: "secret",
		Runner: func(context.Context, string, string, string, string) error {
			t.Fatal("runner should not be called")
			return nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/update", strings.NewReader(`{"service":"clirelay"}`))
	rec := httptest.NewRecorder()

	server.handleUpdate(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestUpdaterPersistsRequestedImageBeforeComposeUpdate(t *testing.T) {
	envFile := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envFile, []byte("CLI_PROXY_IMAGE=ghcr.io/kittors/clirelay:dev\nOTHER=value\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	called := make(chan struct{}, 1)
	server := newUpdaterServer(updaterConfig{
		EnvFile: envFile,
		Runner: func(_ context.Context, _ string, _ string, _ string, _ string) error {
			data, err := os.ReadFile(envFile)
			if err != nil {
				t.Errorf("read env file: %v", err)
			}
			content := string(data)
			if !strings.Contains(content, "CLI_PROXY_IMAGE=ghcr.io/kittors/clirelay:latest\n") {
				t.Errorf("env file content = %q, want requested latest image persisted", content)
			}
			if !strings.Contains(content, "OTHER=value\n") {
				t.Errorf("env file content = %q, want unrelated values preserved", content)
			}
			called <- struct{}{}
			return nil
		},
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/update",
		strings.NewReader(`{"service":"cli-proxy-api","image":"ghcr.io/kittors/clirelay","tag":"latest"}`),
	)
	rec := httptest.NewRecorder()

	server.handleUpdate(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner")
	}
}

func TestUpdaterRejectsRequestWhenEnvFileCannotBeUpdated(t *testing.T) {
	envDir := filepath.Join(t.TempDir(), "readonly")
	if err := os.Mkdir(envDir, 0o500); err != nil {
		t.Fatalf("make readonly dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(envDir, 0o700)
	})

	server := newUpdaterServer(updaterConfig{
		EnvFile: filepath.Join(envDir, ".env"),
		Runner: func(_ context.Context, _ string, _ string, _ string, _ string) error {
			t.Fatal("runner should not be called when env file cannot be updated")
			return nil
		},
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/update",
		strings.NewReader(`{"service":"cli-proxy-api","image":"ghcr.io/kittors/clirelay","tag":"dev"}`),
	)
	rec := httptest.NewRecorder()

	server.handleUpdate(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "failed to update env file") {
		t.Fatalf("body = %q, want env update failure", rec.Body.String())
	}
}

func TestUpdaterAcceptsRequestAndRunsComposeUpdate(t *testing.T) {
	called := make(chan string, 1)
	server := newUpdaterServer(updaterConfig{
		Token:          "secret",
		ComposeFile:    "/workspace/docker-compose.yml",
		EnvFile:        "/workspace/.env",
		ProjectName:    "cliproxy",
		DefaultService: "clirelay",
		Runner: func(_ context.Context, composeFile string, envFile string, projectName string, service string) error {
			if composeFile != "/workspace/docker-compose.yml" {
				t.Errorf("composeFile = %q", composeFile)
			}
			if envFile != "/workspace/.env" {
				t.Errorf("envFile = %q", envFile)
			}
			if projectName != "cliproxy" {
				t.Errorf("projectName = %q", projectName)
			}
			called <- service
			return nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/update", strings.NewReader(`{"service":"cli-proxy-api"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	server.handleUpdate(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	select {
	case service := <-called:
		if service != "cli-proxy-api" {
			t.Fatalf("service = %q, want cli-proxy-api", service)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner")
	}
}

func TestBuildComposeArgsIncludesProjectName(t *testing.T) {
	args := buildComposeArgs(
		"/workspace/docker-compose.yml",
		"/workspace/.env",
		"cliproxy",
		"up",
		"-d",
		"cli-proxy-api",
	)

	want := []string{
		"compose",
		"--project-name", "cliproxy",
		"--env-file", "/workspace/.env",
		"-f", "/workspace/docker-compose.yml",
		"up", "-d", "cli-proxy-api",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
}
