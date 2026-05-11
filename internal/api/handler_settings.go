package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/models"
	"shellyadmin/internal/services"
)

// GetSettings returns the persisted AppSettings with MCPToken redacted
// to MCPTokenRedacted ("<set>") when non-empty. The SPA round-trips that
// placeholder on save; SaveSettings interprets it as "keep existing
// token unchanged."
func (h *Handler) GetSettings(c *gin.Context) {
	settings, err := h.service.GetSettings()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	if settings.MCPToken != "" {
		settings.MCPToken = services.MCPTokenRedacted
	}
	// Runtime overlays for the UI: env-managed state + live listener
	// status. Neither is persisted.
	settings.MCPManagedByEnv = h.service.MCPManagedByEnv()
	settings.MCPRunning = h.service.MCPRunning()
	c.JSON(http.StatusOK, settings)
}

// SaveSettings persists the operator-supplied AppSettings. The
// service-layer Normalize() clamps numeric ranges; ValidateSettings
// rejects invalid MCP token formats and compliance regexes before
// the row is written.
func (h *Handler) SaveSettings(c *gin.Context) {
	var settings models.AppSettings
	if err := decodeJSON(c, &settings, 64*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid settings"})
		return
	}
	if err := h.service.SaveSettings(settings); err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
