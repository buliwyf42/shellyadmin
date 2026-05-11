package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"shellyadmin/internal/db"
	"shellyadmin/internal/middleware"
	"shellyadmin/internal/services"
)

// sessionRouterSetup builds the full Login→RequireAuth→Logout chain
// the production router exposes, so end-to-end tests can exercise S5
// (server-side session revocation): a cookie that was valid before
// logout must be refused afterwards even though its 7-day MaxAge has
// not elapsed.
func sessionRouterSetup(t *testing.T) (*gin.Engine, *Handler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ensureSecretboxKey(t)

	dataDir := t.TempDir()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	hash, err := services.HashPassword("correct-horse")
	if err != nil {
		t.Fatalf("HashPassword error = %v", err)
	}
	cfg := Config{User: "admin", PassHash: hash, Secret: "test-secret", DataDir: dataDir}
	h := NewHandler(database, cfg)

	r := gin.New()
	store := cookie.NewStore([]byte(cfg.Secret))
	r.Use(sessions.Sessions("shellyadmin", store))
	validator := h.service.SessionValidator()
	r.POST("/api/login", h.Login)
	r.POST("/api/logout", middleware.RequireAuth(validator), h.Logout)
	// A protected resource for the regression check. /api/version
	// requires auth in production.
	r.GET("/api/version", middleware.RequireAuth(validator), h.Version)
	return r, h
}

func postJSON(r *gin.Engine, method, path string, body any, cookieHeader string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestServerSessionRevokedAfterLogout is the S5 contract: a cookie
// captured BEFORE the operator clicks Logout must be refused AFTER.
// Pre-S5 the cookie remained valid for the full MaxAge window, which
// the security review flagged as the highest-impact cookie-theft
// persistence vector.
func TestServerSessionRevokedAfterLogout(t *testing.T) {
	r, _ := sessionRouterSetup(t)

	// 1) Login → capture session cookie.
	loginRec := postJSON(r, "POST", "/api/login",
		map[string]string{"username": "admin", "password": "correct-horse"}, "")
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200, body=%s", loginRec.Code, loginRec.Body.String())
	}
	cookieHeader := strings.Split(loginRec.Header().Get("Set-Cookie"), ";")[0]
	if !strings.HasPrefix(cookieHeader, "shellyadmin=") {
		t.Fatalf("expected shellyadmin cookie, got %q", cookieHeader)
	}

	// 2) Cookie works on a protected endpoint.
	ok := postJSON(r, "GET", "/api/version", nil, cookieHeader)
	if ok.Code != http.StatusOK {
		t.Fatalf("pre-logout protected GET = %d, want 200, body=%s", ok.Code, ok.Body.String())
	}

	// 3) Logout with the same cookie.
	out := postJSON(r, "POST", "/api/logout", nil, cookieHeader)
	if out.Code != http.StatusOK {
		t.Fatalf("logout = %d, want 200, body=%s", out.Code, out.Body.String())
	}

	// 4) The captured cookie must now be refused. This is the S5
	//    guarantee — without it the cookie would still be honoured for
	//    its full 7-day MaxAge.
	after := postJSON(r, "GET", "/api/version", nil, cookieHeader)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout GET = %d, want 401 (session revoked), body=%s",
			after.Code, after.Body.String())
	}
}

// TestRequireAuthRejectsCookieWithoutSessionID guards against a
// regression where Login forgets to issue a server-side row. A cookie
// that has "user" set but no "session_id" is treated as a pre-S5
// artefact and refused.
func TestRequireAuthRejectsCookieWithoutSessionID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dataDir := t.TempDir()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	cfg := Config{User: "admin", Secret: "test-secret", DataDir: dataDir}
	h := NewHandler(database, cfg)

	r := gin.New()
	store := cookie.NewStore([]byte(cfg.Secret))
	r.Use(sessions.Sessions("shellyadmin", store))
	validator := h.service.SessionValidator()
	// Synthetic "set user only" endpoint to simulate a pre-S5 cookie.
	r.POST("/legacy-login", func(c *gin.Context) {
		sess := sessions.Default(c)
		sess.Set("user", "admin")
		_ = sess.Save()
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	r.GET("/api/version", middleware.RequireAuth(validator), h.Version)

	leg := postJSON(r, "POST", "/legacy-login", nil, "")
	cookieHeader := strings.Split(leg.Header().Get("Set-Cookie"), ";")[0]
	got := postJSON(r, "GET", "/api/version", nil, cookieHeader)
	if got.Code != http.StatusUnauthorized {
		t.Fatalf("legacy cookie should be refused, got %d", got.Code)
	}
}
