package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/services"
)

// ChangeCredentials updates the operator's username and/or password after
// verifying the current password. Cookie-only (a PAT must not be able to
// rotate the login that gates it — same privilege-escalation guard as the
// token-management endpoints). On success every session for the operator is
// revoked so a stolen cookie cannot outlive the rotation; the SPA redirects
// to the login screen.
func (h *Handler) ChangeCredentials(c *gin.Context) {
	currentUser, ok := h.requirePATCallerCookieOnly(c)
	if !ok {
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		Username        string `json:"username"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(c, &req, 4*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	_, adminHash, configured := h.adminCredential()
	if !configured {
		c.JSON(http.StatusConflict, gin.H{"error": "not configured"})
		return
	}
	if verified, _ := services.VerifyPassword(req.CurrentPassword, adminHash); !verified {
		h.logReq(c, "WARN", "credential change rejected: wrong current password")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "current password incorrect"})
		return
	}
	if len(req.NewPassword) < minOperatorPasswordLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return
	}
	newUsername := strings.TrimSpace(req.Username)
	if newUsername == "" {
		newUsername = currentUser
	}
	if err := h.service.ChangeAdminCredential(newUsername, req.NewPassword); err != nil {
		h.respondError(c, http.StatusInternalServerError, "credential change failed", err)
		return
	}
	// Rotate sessions so the old cookie is dead after a password change.
	// Cover both the old and (when renamed) new username keys.
	_ = h.service.RevokeSessionsForUser(currentUser)
	if newUsername != currentUser {
		_ = h.service.RevokeSessionsForUser(newUsername)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
