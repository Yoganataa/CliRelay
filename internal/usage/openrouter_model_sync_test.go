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

func TestSyncOpenRouterModelsSkipsExistingUserModels(t *testing.T) {
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
				Prompt:     "0.00000175",
				Completion: "0.000014",
			},
		},
	})
	if err != nil {
		t.Fatalf("SyncOpenRouterModelList() error = %v", err)
	}
	if result.Seen != 1 || result.Added != 0 || result.Skipped != 1 {
		t.Fatalf("unexpected sync result: %+v", result)
	}

	model, ok := GetModelConfig("openai/gpt-5.3-codex")
	if !ok {
		t.Fatal("expected existing model config")
	}
	if model.OwnedBy != "custom-owner" || model.Description != "Local override" || model.InputPricePerMillion != 9 || model.Source != "user" {
		t.Fatalf("existing user model should not be overwritten: %+v", model)
	}
}
