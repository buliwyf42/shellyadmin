package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// runScopeTest wires a minimal gin router with the supplied auth-method
// context + scopes preset on c, then runs the request through
// RequireScope or RequireAnyScope. The handler echoes "ok" so the
// caller can distinguish pass-through (status 200) from middleware
// abort (status 401/403).
func runScopeTest(method, scope string, scopes []string, mw gin.HandlerFunc) *httptest.ResponseRecorder {
	r := gin.New()
	// Pre-auth shim sets the same context keys RequireAuth would set.
	r.Use(func(c *gin.Context) {
		if method != "" {
			c.Set(CtxAuthMethod, method)
		}
		if scopes != nil {
			c.Set(CtxPATScopes, scopes)
		}
		c.Next()
	})
	r.GET("/x", mw, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	return rec
}

// TestRequireScopeCookiePassesThrough — cookie-authed requests always
// pass the scope gate. The scope-gating policy applies only to PATs.
func TestRequireScopeCookiePassesThrough(t *testing.T) {
	rec := runScopeTest(AuthMethodCookie, "devices:read", nil, RequireScope("devices:write"))
	if rec.Code != http.StatusOK {
		t.Errorf("cookie-auth + RequireScope: got %d, want 200", rec.Code)
	}
}

// TestRequireScopePATWithMatchingScope — explicit scope match passes.
func TestRequireScopePATWithMatchingScope(t *testing.T) {
	rec := runScopeTest(AuthMethodPAT, "devices:read", []string{"devices:read"}, RequireScope("devices:read"))
	if rec.Code != http.StatusOK {
		t.Errorf("pat with matching scope: got %d, want 200", rec.Code)
	}
}

// TestRequireScopePATWithAdminWildcard — `admin` scope satisfies any
// RequireScope call. Locks in the wildcard contract.
func TestRequireScopePATWithAdminWildcard(t *testing.T) {
	rec := runScopeTest(AuthMethodPAT, ScopeAdmin, []string{ScopeAdmin}, RequireScope("devices:write"))
	if rec.Code != http.StatusOK {
		t.Errorf("admin scope did not satisfy devices:write: %d", rec.Code)
	}
}

// TestRequireScopePATMissingScope — PAT without the required scope
// gets 403 + the required_scope field in the body so the SPA can
// render a useful error.
func TestRequireScopePATMissingScope(t *testing.T) {
	rec := runScopeTest(AuthMethodPAT, "devices:read", []string{"devices:read"}, RequireScope("devices:write"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing scope: got %d, want 403", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["required_scope"] != "devices:write" {
		t.Errorf("body missing required_scope: %v", body)
	}
}

// TestRequireScopeUnauthenticated — no auth method on the context
// (RequireAuth never ran) gets 401. Defense-in-depth against a routing
// bug landing a RequireScope handler outside the auth group.
func TestRequireScopeUnauthenticated(t *testing.T) {
	rec := runScopeTest("", "", nil, RequireScope("devices:read"))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no auth method: got %d, want 401", rec.Code)
	}
}

// TestRequireAnyScopeAcceptsOneOf — disjunctive variant passes when
// the PAT has at least one of the accepted scopes.
func TestRequireAnyScopeAcceptsOneOf(t *testing.T) {
	rec := runScopeTest(AuthMethodPAT, "", []string{"firmware:read"}, RequireAnyScope("devices:read", "firmware:read"))
	if rec.Code != http.StatusOK {
		t.Errorf("one-of match: got %d, want 200", rec.Code)
	}
}

// TestRequireAnyScopeRejectsNoneOf — disjunctive variant 403s when
// the PAT has none of the accepted scopes.
func TestRequireAnyScopeRejectsNoneOf(t *testing.T) {
	rec := runScopeTest(AuthMethodPAT, "", []string{"settings:read"}, RequireAnyScope("devices:read", "firmware:read"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("none-of match: got %d, want 403", rec.Code)
	}
}

// TestRequireAnyScopeCookiePassesThrough — cookie-authed bypasses the
// disjunctive check just like the single-scope variant.
func TestRequireAnyScopeCookiePassesThrough(t *testing.T) {
	rec := runScopeTest(AuthMethodCookie, "", nil, RequireAnyScope("devices:read"))
	if rec.Code != http.StatusOK {
		t.Errorf("cookie-auth + RequireAnyScope: got %d, want 200", rec.Code)
	}
}

// TestHasScopeInternalHelper — the package-internal hasScope function,
// covered indirectly by the middleware tests above but exercised here
// directly to lock in the wildcard + miss cases without HTTP overhead.
func TestHasScopeInternalHelper(t *testing.T) {
	cases := []struct {
		granted  []string
		required string
		want     bool
	}{
		{[]string{ScopeAdmin}, "devices:read", true},
		{[]string{ScopeAdmin}, "settings:write", true},
		{[]string{"devices:read"}, "devices:read", true},
		{[]string{"devices:read"}, "devices:write", false},
		{[]string{}, "devices:read", false},
		{nil, ScopeAdmin, false},
	}
	for _, c := range cases {
		if got := hasScope(c.granted, c.required); got != c.want {
			t.Errorf("hasScope(%v, %q) = %v, want %v", c.granted, c.required, got, c.want)
		}
	}
}
