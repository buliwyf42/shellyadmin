package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

// fakePATValidator is a stub implementation of PATValidator. The
// LookupPAT function field lets each test case wire its own behavior
// (success / arbitrary error / specific token check) without
// reaching for testify mocks.
type fakePATValidator struct {
	fn func(raw string) (string, []string, error)
}

func (f *fakePATValidator) LookupPAT(raw string) (string, []string, error) {
	return f.fn(raw)
}

// stubValidator is a SessionValidator that always reports the session
// alive — lets the cookie branch tests focus on the cookie state, not
// the session-store interaction.
type stubValidator struct{}

func (stubValidator) ValidateSession(string) (bool, error) { return true, nil }
func (stubValidator) TouchSession(string) error            { return nil }

// buildRouter wires the auth middleware behind a small handler that
// echoes the auth-method + scopes the middleware set on the context.
// Tests assert on the body to confirm the right branch fired.
func buildRouter(patValidator PATValidator) *gin.Engine {
	r := gin.New()
	store := cookie.NewStore([]byte("test-secret"))
	r.Use(sessions.Sessions("shellyadmin", store))
	r.Use(RequireAuthWithPAT(stubValidator{}, patValidator))
	// Path under /api/ so denyAuth returns 401 JSON instead of the
	// HTML-route 302 redirect to /login.
	r.GET("/api/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"method": AuthMethod(c),
			"scopes": PATScopes(c),
		})
	})
	return r
}

// TestExtractBearerSuccess locks in the RFC 6750 prefix-matching
// contract: case-insensitive scheme, single space, token trimmed.
func TestExtractBearerSuccess(t *testing.T) {
	cases := []struct {
		header, want string
	}{
		{"Bearer abc", "abc"},
		{"bearer abc", "abc"}, // case-insensitive scheme
		{"BEARER xyz", "xyz"},
		{"Bearer   spaces  ", "spaces"}, // trimmed
	}
	for _, c := range cases {
		r := gin.New()
		var got string
		var ok bool
		r.GET("/x", func(ctx *gin.Context) {
			got, ok = extractBearer(ctx)
		})
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Authorization", c.header)
		r.ServeHTTP(httptest.NewRecorder(), req)
		if !ok {
			t.Errorf("extractBearer(%q): ok=false, want true", c.header)
		}
		if got != c.want {
			t.Errorf("extractBearer(%q) = %q, want %q", c.header, got, c.want)
		}
	}
}

// TestExtractBearerFailures locks in the rejection shapes: missing
// header, wrong scheme, empty token.
func TestExtractBearerFailures(t *testing.T) {
	cases := []string{
		"",          // no header
		"Basic abc", // wrong scheme
		"Bearer",    // missing space + token
		"Bearer ",   // empty token after trim
	}
	for _, h := range cases {
		r := gin.New()
		var ok bool
		r.GET("/x", func(ctx *gin.Context) {
			_, ok = extractBearer(ctx)
		})
		req := httptest.NewRequest("GET", "/x", nil)
		if h != "" {
			req.Header.Set("Authorization", h)
		}
		r.ServeHTTP(httptest.NewRecorder(), req)
		if ok {
			t.Errorf("extractBearer(%q): ok=true, want false", h)
		}
	}
}

// TestPATAuthSucceeds — valid bearer token routes through the PAT
// branch and sets CtxAuthMethod=pat + CtxPATScopes on the context.
func TestPATAuthSucceeds(t *testing.T) {
	validator := &fakePATValidator{
		fn: func(raw string) (string, []string, error) {
			if raw != "pat_xyz" {
				t.Errorf("LookupPAT got %q", raw)
			}
			return "admin", []string{"devices:read"}, nil
		},
	}
	r := buildRouter(validator)
	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer pat_xyz")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PAT auth: status=%d, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); !contains(got, `"method":"pat"`) {
		t.Errorf("response missing method=pat: %s", got)
	}
	if !contains(rec.Body.String(), `"scopes":["devices:read"]`) {
		t.Errorf("response missing scopes: %s", rec.Body.String())
	}
}

// TestPATAuthRejectsInvalidToken — when LookupPAT returns ErrInvalidToken
// (or any error), the middleware returns 401 with a uniform body so an
// attacker can't differentiate the failure mode from the response.
func TestPATAuthRejectsInvalidToken(t *testing.T) {
	validator := &fakePATValidator{
		fn: func(string) (string, []string, error) {
			return "", nil, errors.New("malformed")
		},
	}
	r := buildRouter(validator)
	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer pat_bad")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad PAT: status=%d, want 401", rec.Code)
	}
}

// TestNoBearerFallsThroughToCookie — without an Authorization header,
// the middleware falls through to the cookie path. No cookie present
// → 401 cookie-denial (the existing pre-PAT behavior).
func TestNoBearerFallsThroughToCookie(t *testing.T) {
	validator := &fakePATValidator{
		fn: func(string) (string, []string, error) {
			t.Errorf("LookupPAT called when no bearer header present")
			return "", nil, nil
		},
	}
	r := buildRouter(validator)
	req := httptest.NewRequest("GET", "/api/protected", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no-auth fall-through: status=%d, want 401", rec.Code)
	}
}

// TestNilPATValidator — passing nil for the PAT validator falls back
// to cookie-only auth. Bearer header is ignored; the request goes
// down the cookie path.
func TestNilPATValidator(t *testing.T) {
	r := buildRouter(nil)
	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer pat_xyz")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("nil patValidator + bearer + no cookie: status=%d, want 401", rec.Code)
	}
}

// TestAuthMethodReader — AuthMethod returns the stored string, or ""
// when the context has no auth-method tag.
func TestAuthMethodReader(t *testing.T) {
	r := gin.New()
	var got string
	r.GET("/none", func(c *gin.Context) {
		got = AuthMethod(c)
		c.Status(200)
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/none", nil))
	if got != "" {
		t.Errorf("AuthMethod with no tag: %q, want empty", got)
	}

	r = gin.New()
	r.GET("/cookie", func(c *gin.Context) {
		c.Set(CtxAuthMethod, AuthMethodCookie)
		got = AuthMethod(c)
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/cookie", nil))
	if got != AuthMethodCookie {
		t.Errorf("AuthMethod after set: %q, want %q", got, AuthMethodCookie)
	}
}

// TestPATScopesReader — PATScopes returns the stored slice or nil.
func TestPATScopesReader(t *testing.T) {
	r := gin.New()
	var got []string
	r.GET("/x", func(c *gin.Context) {
		c.Set(CtxPATScopes, []string{"a", "b"})
		got = PATScopes(c)
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("PATScopes = %v, want [a b]", got)
	}
}

// TestErrPATUnsupportedConstant — defense in depth that the exported
// sentinel hasn't been removed. Callers may switch on errors.Is.
func TestErrPATUnsupportedConstant(t *testing.T) {
	if ErrPATUnsupported == nil {
		t.Errorf("ErrPATUnsupported was nilled out")
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
