package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/models"
)

// ListCredentials returns every stored credential row. Password / HA1
// fields are decrypted before serialisation; the SPA receives them in
// plaintext because the credentials *are* the secret payload the
// operator manages.
func (h *Handler) ListCredentials(c *gin.Context) {
	credentials, err := h.service.ListCredentials()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, credentials)
}

// SaveCredential persists a credential (upsert by name).
func (h *Handler) SaveCredential(c *gin.Context) {
	var req models.Credential
	if err := decodeJSON(c, &req, 64*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential"})
		return
	}
	if err := h.service.SaveCredential(req); err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteCredential removes a credential by name. Refuses if any
// credential group still references the credential.
func (h *Handler) DeleteCredential(c *gin.Context) {
	if err := h.service.DeleteCredential(c.Param("name")); err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListCredentialGroups returns every stored credential-group row with
// the embedded password / HA1 decrypted.
func (h *Handler) ListCredentialGroups(c *gin.Context) {
	groups, err := h.service.ListCredentialGroups()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, groups)
}

// SaveCredentialGroup persists a credential group (upsert by name).
func (h *Handler) SaveCredentialGroup(c *gin.Context) {
	var req models.CredentialGroup
	if err := decodeJSON(c, &req, 32*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group"})
		return
	}
	if err := h.service.SaveCredentialGroup(req); err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteCredentialGroup removes a credential group by name and the
// device-assignment rows that reference it (cascaded inside a single
// transaction).
func (h *Handler) DeleteCredentialGroup(c *gin.Context) {
	if err := h.service.DeleteCredentialGroup(c.Param("name")); err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetCredentialGroupAssignments returns every (MAC, group_name) pair.
func (h *Handler) GetCredentialGroupAssignments(c *gin.Context) {
	assignments, err := h.service.ListCredentialGroupAssignments()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"assignments": assignments})
}

// SaveCredentialGroupAssignments assigns each MAC to the supplied
// group_name (or empty to clear).
func (h *Handler) SaveCredentialGroupAssignments(c *gin.Context) {
	var req struct {
		MACs      []string `json:"macs"`
		GroupName string   `json:"group_name"`
	}
	if err := decodeJSON(c, &req, 128*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid assignment request"})
		return
	}
	if err := h.service.SaveCredentialGroupAssignments(req.MACs, req.GroupName); err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
