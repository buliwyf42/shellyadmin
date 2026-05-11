package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ScopeAdmin is the wildcard scope that satisfies every RequireScope
// check. Kept here as a string literal so this package doesn't have
// to import the tokens sub-package (it's a leaf for the auth chain).
const ScopeAdmin = "admin"

// RequireScope enforces per-route scope authorization for PAT-authed
// requests. Cookie-authed requests pass through — a logged-in
// operator implicitly has every scope; the only authorization
// granularity in v0.3.0 lives on the PAT side.
//
// Behavior:
//   - Auth method == cookie: pass through.
//   - Auth method == pat AND scopes contain `required` OR `admin`: pass.
//   - Auth method == pat AND scopes do NOT contain `required`: 403.
//   - Auth method == "" (the auth middleware never ran): 401. This is
//     defense-in-depth — a routing bug that landed a RequireScope
//     handler outside the auth group would otherwise pass silently.
//
// The required-scope argument is a single string. Routes that need
// "either of two scopes" should call RequireAnyScope instead; the
// catalog deliberately doesn't have OR semantics in the common case.
func RequireScope(required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := AuthMethod(c)
		switch method {
		case AuthMethodCookie:
			c.Next()
			return
		case AuthMethodPAT:
			if hasScope(PATScopes(c), required) {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":          "missing scope",
				"required_scope": required,
			})
			return
		default:
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
	}
}

// RequireAnyScope is the disjunctive variant — pass if the PAT carries
// ANY of `accepted`. Used by routes that have a "read OR write"
// authorization model (rare; most routes are single-scope).
func RequireAnyScope(accepted ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := AuthMethod(c)
		switch method {
		case AuthMethodCookie:
			c.Next()
			return
		case AuthMethodPAT:
			granted := PATScopes(c)
			for _, want := range accepted {
				if hasScope(granted, want) {
					c.Next()
					return
				}
			}
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":           "missing scope",
				"accepted_scopes": accepted,
			})
			return
		default:
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
	}
}

// hasScope is the policy helper. Inlined here (not re-exported from
// the tokens sub-package) so middleware stays a leaf module — the
// build cycle is auth → middleware → tokens, and middleware should
// not pull tokens upward.
func hasScope(granted []string, required string) bool {
	for _, sc := range granted {
		if sc == ScopeAdmin || sc == required {
			return true
		}
	}
	return false
}
