package util

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
)

func TestGetProviderNameResolvesGPTImage2FromCodexRegistration(t *testing.T) {
	reg := registry.GetGlobalRegistry()
	clientID := "test-codex-gpt-image-2-provider"
	reg.RegisterClient(clientID, "codex", registry.GetOpenAIModels())
	t.Cleanup(func() { reg.UnregisterClient(clientID) })

	providers := GetProviderName("gpt-image-2")
	for _, provider := range providers {
		if provider == "codex" {
			return
		}
	}

	t.Fatalf("expected gpt-image-2 providers to include codex, got %#v", providers)
}
