package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// SessionValidator is the narrow seam the auth middleware uses to check
// a server-side session record. Implementations (typically *db.DB)
// return:
//   - nil + ok=true: session row exists, not revoked, not expired.
//   - nil + ok=false: session row missing, revoked, or expired. The
//     middleware will refuse the request and clear the cookie.
//   - non-nil err: storage error. Treat as "auth failed" so a flaky
//     DB does not hand out implicit anonymous access.
//
// TouchSession is best-effort — middleware logs but does not fail on
// errors so a slow write does not deny an otherwise-valid request.
type SessionValidator interface {
	ValidateSession(id string) (ok bool, err error)
	TouchSession(id string) error
}

// RequireAuth gates routes behind a valid session. It performs three
// independent checks:
//
//  1. The session cookie carries a "user" claim (legacy gin-contrib
//     check — covers the case where someone bypassed Login entirely).
//  2. The cookie also carries a "session_id" — Login always sets this
//     in tandem with "user" after S5. Missing means a pre-S5 cookie or
//     a tampered one.
//  3. The validator (server-side store) confirms the session is alive.
//
// Anything that fails clears the cookie + redirects to /login (HTML
// routes) or returns 401 JSON (/api routes).
func RequireAuth(validator SessionValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		if session.Get("user") == nil {
			denyAuth(c, session, "authentication required")
			return
		}
		sid, _ := session.Get("session_id").(string)
		if sid == "" {
			// Pre-S5 cookie (no session_id). Treat as logged-out so the
			// client gets a fresh login under the new pipeline.
			denyAuth(c, session, "session refresh required")
			return
		}
		if validator != nil {
			ok, err := validator.ValidateSession(sid)
			if err != nil || !ok {
				denyAuth(c, session, "session revoked or expired")
				return
			}
			// Best-effort touch — never blocks the request.
			_ = validator.TouchSession(sid)
		}
		c.Next()
	}
}

func denyAuth(c *gin.Context, session sessions.Session, reason string) {
	session.Clear()
	session.Options(sessions.Options{
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	_ = session.Save()
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": reason})
		return
	}
	c.Redirect(http.StatusFound, "/login")
	c.Abort()
}

// SessionLifetime is the absolute expiry stamped on a fresh session
// row. Sliding-window expiry (extending it on each touch) is a Phase 4
// follow-up; the current model is a hard 7-day cap matching the cookie
// MaxAge in router.go.
const SessionLifetime = 7 * 24 * time.Hour
