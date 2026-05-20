package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/services"
)

// minOperatorPasswordLen is the floor for a new operator password set via
// first-run setup or the change-credentials flow. argon2id has no inherent
// length requirement; this is a usability guard against a fat-fingered empty
// or one-character password locking the operator into a trivial credential.
const minOperatorPasswordLen = 8

// SetupStatusResponse is the GET /api/setup/status body. It carries ONLY a
// boolean — never any credential material — because it is served on the
// public (pre-auth) surface so the SPA can choose between the setup screen
// and the login screen on first load.
type SetupStatusResponse struct {
	Configured bool `json:"configured"`
}

// SetupStatus reports whether an operator login exists yet. Public: the SPA
// hits this before authenticating to decide whether to render setup or login.
func (h *Handler) SetupStatus(c *gin.Context) {
	_, _, configured := h.adminCredential()
	c.JSON(http.StatusOK, SetupStatusResponse{Configured: configured})
}

// Setup is the first-run path: with no operator login configured, accept a
// username + password and persist them (argon2id-hashed) as the operator
// account. Public + rate-limited (the only unauthenticated mutation). Once an
// account exists it returns 409 so a second caller cannot overwrite it.
func (h *Handler) Setup(c *gin.Context) {
	if _, _, configured := h.adminCredential(); configured {
		c.JSON(http.StatusConflict, gin.H{"error": "already configured"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(c, &req, 4*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if len(req.Password) < minOperatorPasswordLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return
	}
	if err := h.service.SetupAdminCredential(req.Username, req.Password); err != nil {
		if errors.Is(err, services.ErrAuthAlreadyConfigured) {
			c.JSON(http.StatusConflict, gin.H{"error": "already configured"})
			return
		}
		h.respondError(c, http.StatusInternalServerError, "setup failed", err)
		return
	}
	h.logReq(c, "WARN", "first-run setup completed")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
