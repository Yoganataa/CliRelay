package management

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
)

// GetModels returns the list of all available models from the global registry.
// This endpoint is protected by the management middleware, so no external API key is needed.
//
// Endpoint:
//
//	GET /v0/management/models
func (h *Handler) GetModels(c *gin.Context) {
	modelRegistry := registry.GetGlobalRegistry()
	allModels := modelRegistry.GetAvailableModels("openai")

	// Filter to only include the 4 required fields: id, object, created, owned_by
	filteredModels := make([]map[string]any, len(allModels))
	for i, model := range allModels {
		filteredModel := map[string]any{
			"id":     model["id"],
			"object": model["object"],
		}
		if created, exists := model["created"]; exists {
			filteredModel["created"] = created
		}
		if ownedBy, exists := model["owned_by"]; exists {
			filteredModel["owned_by"] = ownedBy
		}
		filteredModels[i] = filteredModel
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   filteredModels,
	})
}
