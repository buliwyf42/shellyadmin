package api

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"shellyadmin/internal/services"
)

// Login authenticates the operator against the configured username +
// argon2id PHC hash, issues a server-side session row (S5), and returns
// the CSRF nonce in the response body. Failed attempts feed the
// per-account lockout counter (Q20); account-locked responses use 423.
func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(c, &req, 4*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	// Username + password must each be evaluated to a boolean BEFORE the
	// short-circuit `||` collapses them — otherwise verifyAdminPassword
	// (argon2id, ~80 ms) would be skipped on a username mismatch, giving
	// the attacker a timing oracle to enumerate valid usernames. Running
	// argon2 unconditionally also pads the response on a missing-user case.
	unameOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(h.cfg.User)) == 1
	pwOK := h.verifyAdminPassword(c, req.Password)

	// Account-lockout (Q20) is checked AFTER argon2 to keep the response
	// timing flat across locked/unlocked states. The configured username
	// from cfg.User is the canonical key — using the submitted username
	// would let an attacker probe arbitrary usernames into the lockout
	// table. In the Single-Operator model, only h.cfg.User can ever lock.
	locked, until := h.service.IsAccountLocked(h.cfg.User)
	if locked {
		h.logReq(c, "WARN", fmt.Sprintf("login blocked: account locked until %s", until.Format(time.RFC3339)))
		c.Header("Retry-After", fmt.Sprintf("%d", int(time.Until(until).Seconds())))
		c.JSON(http.StatusLocked, gin.H{
			"error":       "account locked due to repeated failed logins",
			"retry_after": until.UTC().Format(time.RFC3339),
		})
		return
	}

	if !unameOK || !pwOK {
		if err := h.service.RecordLoginFailure(h.cfg.User); err != nil {
			h.logReq(c, "ERROR", fmt.Sprintf("record login failure: %v", err))
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if err := h.service.RecordLoginSuccess(h.cfg.User); err != nil {
		h.logReq(c, "WARN", fmt.Sprintf("record login success: %v", err))
	}
	session := sessions.Default(c)
	session.Clear()
	session.Set("user", req.Username)
	nonce := RandomSecret()
	session.Set("nonce", nonce)
	// S5 — every successful login issues a fresh server-side session row.
	// The cookie carries only the opaque id; the authoritative state
	// (revoked? expired? owner?) lives in the DB. RequireAuth on every
	// subsequent request consults the row, so a stolen cookie is
	// invalidated by the operator clicking Logout — pre-S5 the cookie
	// remained valid for its full 7-day MaxAge.
	sessionID := RandomSecret()
	session.Set("session_id", sessionID)
	if _, err := h.service.IssueSession(sessionID, req.Username); err != nil {
		h.logReq(c, "ERROR", fmt.Sprintf("login: issue session row failed: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session persistence failed"})
		return
	}
	if err := session.Save(); err != nil {
		h.logReq(c, "ERROR", fmt.Sprintf("login: session save failed: %v", err))
		_ = h.service.RevokeSession(sessionID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session persistence failed"})
		return
	}
	// Only the JSON body carries the token now. Phase 1 Q12 removed the
	// `X-CSRF-Token` response header (was echoed by RequireCSRF on every
	// authenticated GET, turning any DOM-injection sink in the SPA into
	// a trivial CSRF bypass via `fetch('/api/...').then(r =>
	// r.headers.get('X-CSRF-Token'))`).
	c.JSON(http.StatusOK, gin.H{"ok": true, "csrf_token": nonce})
}

// Logout revokes the server-side session row, then clears the cookie.
// Order matters: revoke FIRST so a race where the cookie clears but the
// row stays active cannot leave a stolen cookie valid.
func (h *Handler) Logout(c *gin.Context) {
	session := sessions.Default(c)
	if sid, ok := session.Get("session_id").(string); ok && sid != "" && h.service != nil {
		if err := h.service.RevokeSession(sid); err != nil {
			h.logReq(c, "WARN", fmt.Sprintf("logout: revoke session failed: %v", err))
		}
	}
	session.Clear()
	session.Options(sessions.Options{Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: h.cfg.CookieSecure})
	if err := session.Save(); err != nil {
		// Logout is best-effort: the cookie's MaxAge=-1 already clears
		// the client side, so surface the persistence error to the audit
		// log but still return ok so the user sees a successful sign-out.
		h.logReq(c, "WARN", fmt.Sprintf("logout: session save failed: %v", err))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CSRFToken returns the session nonce in the response body. Used by the
// SPA when its cached token is missing or has been invalidated (e.g.
// after a 401). Phase 1 Q12 removed the previous middleware response-
// header echo; this endpoint is the only authenticated path that
// delivers the nonce.
func (h *Handler) CSRFToken(c *gin.Context) {
	session := sessions.Default(c)
	nonce, _ := session.Get("nonce").(string)
	if nonce == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing session nonce"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"csrf_token": nonce})
}

// verifyAdminPassword checks the supplied plaintext against the
// configured argon2id PHC hash from cfg.PassHash. Always runs the
// argon2 derivation — no short-circuit on empty plain — so the
// response time is independent of input. The Login handler's empty-
// username/password rejection happens AFTER this call, not in place of
// it (see Q11 in the consolidated plan).
func (h *Handler) verifyAdminPassword(c *gin.Context, plain string) bool {
	ok, err := services.VerifyPassword(plain, h.cfg.PassHash)
	if err != nil {
		h.logReq(c, "ERROR", fmt.Sprintf("password hash verify failed: %v", err))
		return false
	}
	return ok
}
