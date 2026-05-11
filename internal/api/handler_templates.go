package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/services"
)

// ListTemplates returns the names of every saved provisioning template.
func (h *Handler) ListTemplates(c *gin.Context) {
	names, err := h.service.ListTemplates()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, names)
}

// GetTemplate returns a single template's content + credential_ref.
func (h *Handler) GetTemplate(c *gin.Context) {
	record, err := h.service.GetTemplate(c.Param("name"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}
	c.JSON(http.StatusOK, record)
}

// SaveTemplate persists a template under the given name (upsert).
func (h *Handler) SaveTemplate(c *gin.Context) {
	var req struct {
		Content       string `json:"content"`
		CredentialRef string `json:"credential_ref"`
	}
	if err := decodeJSON(c, &req, services.MaxTemplateBytes+1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content required"})
		return
	}
	if err := h.service.SaveTemplate(c.Param("name"), req.Content, req.CredentialRef); err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteTemplate removes a template by name. Idempotent: deleting a
// non-existent template still returns 200.
func (h *Handler) DeleteTemplate(c *gin.Context) {
	if err := h.service.DeleteTemplate(c.Param("name")); err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
