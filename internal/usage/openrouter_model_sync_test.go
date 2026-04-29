package usage

import (
	"context"
	"testing"
)

func TestSyncOpenRouterModelsAddsNewModelsWithPricingAndOwner(t *testing.T) {
	initModelConfigTestDB(t)

	result, err := SyncOpenRouterModelList(context.Background(), []OpenRouterRemoteModel{
		{
			ID:          "openai/gpt-5.3-codex",
			Name:        "OpenAI: GPT-5.3-Codex",
			Description: "Agentic coding model",
			Pricing: OpenRouterRemotePricing{
				Prompt:         "0.00000175",
				Completion:     "0.000014",
				InputCacheRead: "0.000000175",
			},
		},
	})
	if err != nil {
		t.Fatalf("SyncOpenRouterModelList() error = %v", err)
	}
	if result.Seen != 1 || result.Added != 1 || result.Skipped != 0 {
		t.Fatalf("unexpected sync result: %+v", result)
	}

	model, ok := GetModelConfig("openai/gpt-5.3-codex")
	if !ok {
		t.Fatal("expected openai/gpt-5.3-codex to be imported")
	}
	if model.OwnedBy != "openai" || model.Source != "openrouter" || model.Description != "Agentic coding model" {
		t.Fatalf("unexpected imported model metadata: %+v", model)
	}
	if model.InputPricePerMillion != 1.75 || model.OutputPricePerMillion != 14 || model.CachedPricePerMillion != 0.175 {
		t.Fatalf("unexpected imported model pricing: %+v", model)
	}
	if _, ok := GetModelOwnerPreset("openai"); !ok {
		t.Fatal("expected openai owner preset to exist")
	}
}

func TestSyncOpenRouterModelsUpdatesExistingUserModelPricingOnly(t *testing.T) {
	initModelConfigTestDB(t)

	if err := UpsertModelConfig(ModelConfigRow{
		ModelID:               "openai/gpt-5.3-codex",
		OwnedBy:               "custom-owner",
		Description:           "Local override",
		Enabled:               true,
		PricingMode:           "token",
		InputPricePerMillion:  9,
		OutputPricePerMillion: 18,
		Source:                "user",
	}); err != nil {
		t.Fatalf("UpsertModelConfig() error = %v", err)
	}

	result, err := SyncOpenRouterModelList(context.Background(), []OpenRouterRemoteModel{
		{
			ID:          "openai/gpt-5.3-codex",
			Description: "Remote description",
			Pricing: OpenRouterRemotePricing{
				Prompt:         "0.00000175",
				Completion:     "0.000014",
				InputCacheRead: "0.000000175",
			},
		},
	})
	if err != nil {
		t.Fatalf("SyncOpenRouterModelList() error = %v", err)
	}
	if result.Seen != 1 || result.Added != 0 || result.Updated != 1 || result.Skipped != 0 {
		t.Fatalf("unexpected sync result: %+v", result)
	}

	model, ok := GetModelConfig("openai/gpt-5.3-codex")
	if !ok {
		t.Fatal("expected existing model config")
	}
	if model.OwnedBy != "custom-owner" || model.Description != "Local override" || model.Source != "user" {
		t.Fatalf("existing user metadata should not be overwritten: %+v", model)
	}
	if model.InputPricePerMillion != 1.75 || model.OutputPricePerMillion != 14 || model.CachedPricePerMillion != 0.175 {
		t.Fatalf("existing user model pricing should be synced: %+v", model)
	}
}

func TestSyncOpenRouterModelsStripsTildeFromOwnerPrefix(t *testing.T) {
	initModelConfigTestDB(t)

	result, err := SyncOpenRouterModelList(context.Background(), []OpenRouterRemoteModel{
		{
			ID:          "~moonshotai/kimi-latest",
			Description: "Moonshot latest alias",
			Pricing: OpenRouterRemotePricing{
				Prompt:     "0.0000007448",
				Completion: "0.000004655",
			},
		},
	})
	if err != nil {
		t.Fatalf("SyncOpenRouterModelList() error = %v", err)
	}
	if result.Seen != 1 || result.Added != 1 || result.Updated != 0 || result.Skipped != 0 {
		t.Fatalf("unexpected sync result: %+v", result)
	}

	model, ok := GetModelConfig("~moonshotai/kimi-latest")
	if !ok {
		t.Fatal("expected OpenRouter alias model to be imported with its original id")
	}
	if model.ModelID != "~moonshotai/kimi-latest" {
		t.Fatalf("model id should remain the OpenRouter id, got %q", model.ModelID)
	}
	if model.OwnedBy != "moonshotai" {
		t.Fatalf("owner should not keep OpenRouter alias marker, got %q", model.OwnedBy)
	}
}

func TestSyncOpenRouterModelsCleansExistingTildeOwnerWhenUpdatingPricing(t *testing.T) {
	initModelConfigTestDB(t)

	if err := UpsertModelConfig(ModelConfigRow{
		ModelID:               "~moonshotai/kimi-latest",
		OwnedBy:               "～moonshotai",
		Description:           "Existing imported alias",
		Enabled:               true,
		PricingMode:           "token",
		InputPricePerMillion:  9,
		OutputPricePerMillion: 18,
		Source:                "openrouter",
	}); err != nil {
		t.Fatalf("UpsertModelConfig() error = %v", err)
	}

	result, err := SyncOpenRouterModelList(context.Background(), []OpenRouterRemoteModel{
		{
			ID:          "~moonshotai/kimi-latest",
			Description: "Remote description",
			Pricing: OpenRouterRemotePricing{
				Prompt:     "0.0000007448",
				Completion: "0.000004655",
			},
		},
	})
	if err != nil {
		t.Fatalf("SyncOpenRouterModelList() error = %v", err)
	}
	if result.Seen != 1 || result.Added != 0 || result.Updated != 1 || result.Skipped != 0 {
		t.Fatalf("unexpected sync result: %+v", result)
	}

	model, ok := GetModelConfig("~moonshotai/kimi-latest")
	if !ok {
		t.Fatal("expected existing model config")
	}
	if model.OwnedBy != "moonshotai" {
		t.Fatalf("existing OpenRouter owner should be cleaned, got %q", model.OwnedBy)
	}
	if model.Description != "Existing imported alias" || model.Source != "openrouter" {
		t.Fatalf("existing OpenRouter metadata should otherwise stay unchanged: %+v", model)
	}
	if model.InputPricePerMillion != 0.7448 || model.OutputPricePerMillion != 4.655 {
		t.Fatalf("existing OpenRouter pricing should be synced: %+v", model)
	}
}
