package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/middleware"
	"shellyadmin/internal/services"
)

// TOKENS handlers — Personal Access Token CRUD (T3, v0.3.0).
// Plaintext token is shown to the operator exactly once on Create and
// never persists outside the response body. The List + Revoke paths
// surface metadata only.
//
// All four endpoints live under the cookie-authenticated SPA chain.
// They are explicitly NOT callable via PAT — a PAT can't mint another
// PAT (RequireScope("admin") on the Create endpoint guards that, but
// the auth model also depends on never letting a PAT escalate
// privileges this way).

// CreateTokenRequest is the POST /api/tokens body shape.
type CreateTokenRequest struct {
	Name          string   `json:"name"`
	Scopes        []string `json:"scopes"`
	ExpiresInDays int      `json:"expires_in_days"`
}

// CreateTokenResponse is what Create returns. Token is shown ONCE.
type CreateTokenResponse = services.PATCreateResult

// ListTokensResponse wraps the array so the response shape is open
// to future metadata fields (e.g. operator-wide PAT count, catalog).
type ListTokensResponse struct {
	Tokens          []services.ListedPAT `json:"tokens"`
	AvailableScopes []string             `json:"available_scopes"`
}

// CreateToken mints a new PAT. The plaintext token surfaces in the
// response body and the caller is expected to copy it to a credential
// store immediately. Subsequent List calls return metadata only.
func (h *Handler) CreateToken(c *gin.Context) {
	username, ok := h.requirePATCallerCookieOnly(c)
	if !ok {
		return
	}
	var req CreateTokenRequest
	if err := decodeJSON(c, &req, 4*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	result, err := h.service.CreatePAT(username, req.Name, req.Scopes, req.ExpiresInDays)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrPATEmptyScopes), errors.Is(err, services.ErrPATInvalidScope):
			h.respondUserError(c, http.StatusBadRequest, err)
			return
		default:
			h.respondUserError(c, http.StatusBadRequest, err)
			return
		}
	}
	h.logReq(c, "INFO", "personal access token created: id="+result.ID+" name="+result.Name)
	c.JSON(http.StatusOK, result)
}

// ListTokens returns the operator's PATs (metadata only). The
// available_scopes field lets the SPA render the create-token form
// without a separate /api/tokens/scopes endpoint.
//
// Cookie-only: PATs are not allowed to enumerate other PATs (would
// leak the inventory of the operator's machine-credentials to anyone
// holding a single token). The Settings UI is the only intended
// caller.
func (h *Handler) ListTokens(c *gin.Context) {
	username, ok := h.requirePATCallerCookieOnly(c)
	if !ok {
		return
	}
	tokens, err := h.service.ListPATs(username)
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "list failed", err)
		return
	}
	c.JSON(http.StatusOK, ListTokensResponse{
		Tokens:          tokens,
		AvailableScopes: services.AllPATScopes(),
	})
}

// RevokeToken marks a PAT revoked. Idempotent — repeated calls return
// 200. The username scope on the service call protects against an
// operator revoking another operator's PAT (future-proofing for the
// multi-user model).
func (h *Handler) RevokeToken(c *gin.Context) {
	username, ok := h.requirePATCallerCookieOnly(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id"})
		return
	}
	if err := h.service.RevokePAT(username, id); err != nil {
		if errors.Is(err, services.ErrPATNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		h.respondError(c, http.StatusInternalServerError, "revoke failed", err)
		return
	}
	h.logReq(c, "INFO", "personal access token revoked: id="+id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// requirePATCallerCookieOnly is a guard for the Create + Revoke paths.
// PATs are not allowed to mint or revoke other PATs — that's an
// explicit privilege escalation surface we close at the handler. A
// PAT-authed request to these endpoints returns 403.
//
// Reads CtxAuthMethod off the gin context; cookie-authed requests
// pass through with the session username. The List endpoint
// deliberately allows PAT access (the auth-method check is absent
// there) so a headless caller can introspect what tokens it has access
// to alongside.
func (h *Handler) requirePATCallerCookieOnly(c *gin.Context) (string, bool) {
	method := middleware.AuthMethod(c)
	if method == middleware.AuthMethodPAT {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": "PAT-authed requests cannot manage personal access tokens",
		})
		return "", false
	}
	username, ok := h.sessionUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return "", false
	}
	return username, true
}
