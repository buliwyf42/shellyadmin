package middleware

import (
	"errors"
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

// PATValidator is the narrow seam the auth middleware uses to verify
// a Personal Access Token bearer string (T3 from the consolidated
// review). Returns:
//   - nil + non-empty username + scopes: token is alive, the request
//     gets the corresponding identity.
//   - non-nil err: malformed / unknown / revoked / expired token. The
//     middleware refuses with 401 (no error-shape differentiation so
//     an attacker can't distinguish missing-id from wrong-random).
//
// Implementations: *services.AppService.LookupPAT.
type PATValidator interface {
	LookupPAT(rawToken string) (username string, scopes []string, err error)
}

// Gin context keys for state set by RequireAuth. Reading these in a
// downstream handler is the supported way to ask "how was this
// request authenticated?".
const (
	// CtxAuthMethod holds "cookie" or "pat".
	CtxAuthMethod = "auth_method"
	// CtxAuthUsername holds the verified principal name.
	CtxAuthUsername = "auth_username"
	// CtxPATScopes holds the PAT's scope list (slice of strings).
	// Absent for cookie-authed requests.
	CtxPATScopes = "pat_scopes"
)

// Auth-method enum values written into CtxAuthMethod.
const (
	AuthMethodCookie = "cookie"
	AuthMethodPAT    = "pat"
)

// RequireAuth gates routes behind a valid authentication mechanism.
// Two paths converge here:
//
//  1. Session cookie (the SPA's path). Three independent checks:
//     - The session cookie carries a "user" claim.
//     - The cookie also carries a "session_id" — Login sets this in
//     tandem with "user" after S5.
//     - The validator (server-side store) confirms the session is alive.
//
//  2. PAT bearer token (the headless-caller path). The Authorization
//     header carries `Bearer pat_...`; patValidator verifies it.
//     PAT-authed requests bypass cookie checks entirely — the bearer
//     token IS the proof of identity. Tested BEFORE cookie because an
//     operator that pastes a PAT into a curl call also has a stale
//     browser cookie around, and we want the explicit token to win.
//
// On success the gin context carries CtxAuthMethod + CtxAuthUsername +
// (for PATs) CtxPATScopes. Downstream middleware (RequireCSRF,
// RequireScope) reads these to decide what to enforce.
//
// patValidator may be nil — that's how tests construct the middleware
// without standing up a PAT service. PAT auth is then unavailable;
// requests carrying `Authorization: Bearer` get 401.
func RequireAuth(validator SessionValidator) gin.HandlerFunc {
	return RequireAuthWithPAT(validator, nil)
}

// RequireAuthWithPAT is the variant that also accepts PAT bearer
// tokens. The two-arg signature exists so production can wire both
// validators while tests can use the single-arg RequireAuth as before.
func RequireAuthWithPAT(validator SessionValidator, patValidator PATValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Bearer token takes precedence over the cookie. An operator
		// running curl with a stale browser cookie should not have the
		// cookie quietly answer for them — the explicit Authorization
		// header is unambiguous intent.
		if patValidator != nil {
			if token, ok := extractBearer(c); ok {
				username, scopes, err := patValidator.LookupPAT(token)
				if err != nil {
					// All PAT failure modes (malformed / unknown id /
					// wrong random / revoked / expired) collapse to a
					// single 401 with the same body so an attacker
					// can't tell which path failed.
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
					return
				}
				c.Set(CtxAuthMethod, AuthMethodPAT)
				c.Set(CtxAuthUsername, username)
				c.Set(CtxPATScopes, scopes)
				c.Next()
				return
			}
		}

		session := sessions.Default(c)
		username, _ := session.Get("user").(string)
		if username == "" {
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
		c.Set(CtxAuthMethod, AuthMethodCookie)
		c.Set(CtxAuthUsername, username)
		c.Next()
	}
}

// extractBearer returns the bearer string from the Authorization
// header. Accepts only the `Bearer ` prefix (case-insensitive on the
// scheme per RFC 6750; the token itself is case-sensitive). Returns
// ok=false when the header is missing or malformed.
func extractBearer(c *gin.Context) (string, bool) {
	h := c.GetHeader("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "bearer "
	if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// AuthMethod reads the auth-method tag the middleware set on the
// context. Returns "" when the context has no tag (handler ran
// outside the auth group, e.g. /api/login itself).
func AuthMethod(c *gin.Context) string {
	v, _ := c.Get(CtxAuthMethod)
	s, _ := v.(string)
	return s
}

// PATScopes reads the PAT scope list off the context. Empty for
// cookie-authed requests.
func PATScopes(c *gin.Context) []string {
	v, _ := c.Get(CtxPATScopes)
	s, _ := v.([]string)
	return s
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

// ErrPATUnsupported is what a nil PATValidator's failure path would
// emit, exported so callers building integration tests can assert on
// it. Currently unused inside the package; future work may surface it.
var ErrPATUnsupported = errors.New("middleware: PAT auth not configured")
