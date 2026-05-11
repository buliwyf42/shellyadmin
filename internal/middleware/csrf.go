package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// RequireCSRF validates the X-CSRF-Token request header against the session
// nonce on state-changing methods. It deliberately does NOT echo the nonce
// back on every response anymore (Q12 in the consolidated review plan):
// echoing on every authenticated GET turned every XSS sink in the SPA into
// a complete CSRF bypass via `fetch('/api/devices').then(r =>
// r.headers.get('X-CSRF-Token'))`. The token is delivered only by the
// dedicated GET /api/csrf-token endpoint and the POST /api/login response
// body, both of which require either a valid session cookie or a fresh
// login — making a stolen XSS sink work harder to retrieve.
func RequireCSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		// PAT-authed requests skip CSRF entirely (T3). The bearer
		// token IS the proof-of-intent — the entire reason CSRF exists
		// is to defend against a victim's browser auto-attaching a
		// cookie cross-origin. Bearer tokens are never auto-attached,
		// so there's no CSRF vector to defend against.
		if AuthMethod(c) == AuthMethodPAT {
			c.Next()
			return
		}
		session := sessions.Default(c)
		nonce, _ := session.Get("nonce").(string)
		if nonce == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing session nonce"})
			return
		}
		token := c.GetHeader("X-CSRF-Token")
		if subtle.ConstantTimeCompare([]byte(token), []byte(nonce)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid csrf token"})
			return
		}
		c.Next()
	}
}
