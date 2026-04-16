package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUpdaterRejectsInvalidBearerToken(t *testing.T) {
	server := newUpdaterServer(updaterConfig{
		Token: "secret",
		Runner: func(context.Context, string, string, string) error {
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

func TestUpdaterAcceptsRequestAndRunsComposeUpdate(t *testing.T) {
	called := make(chan string, 1)
	server := newUpdaterServer(updaterConfig{
		Token:          "secret",
		ComposeFile:    "/workspace/docker-compose.yml",
		EnvFile:        "/workspace/.env",
		DefaultService: "clirelay",
		Runner: func(_ context.Context, composeFile string, envFile string, service string) error {
			if composeFile != "/workspace/docker-compose.yml" {
				t.Errorf("composeFile = %q", composeFile)
			}
			if envFile != "/workspace/.env" {
				t.Errorf("envFile = %q", envFile)
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
