package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/db"
	"shellyadmin/internal/middleware"
	"shellyadmin/internal/services"
)

// tokensTestRouter builds a full router (auth chain + documented
// routes) wired to a fresh SQLite DB. Returns the engine + a cookie
// captured from a successful Login so test cases can mimic an
// authenticated browser session.
func tokensTestRouter(t *testing.T) (*gin.Engine, *http.Cookie, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ensureSecretboxKey(t)
	middleware.ResetRateLimitsForTest()

	dataDir := t.TempDir()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	hash, err := services.HashPassword("correct-horse")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	cfg := Config{
		User:     "admin",
		PassHash: hash,
		Secret:   "test-secret",
		DataDir:  dataDir,
	}
	cfg.Service = services.NewAppService(database, dataDir, func(context.Context, string, string) {})
	r := NewRouter(database, cfg)

	// Login flow via the full router so we get a real session cookie
	// + CSRF token, matching what the SPA does at sign-in.
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "correct-horse"})
	req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	var loginResp struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	var cookieOut *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "shellyadmin" {
			cookieOut = c
		}
	}
	if cookieOut == nil {
		t.Fatalf("no session cookie in login response")
	}
	return r, cookieOut, loginResp.CSRFToken
}

// doRequest is a small helper that runs an authenticated request
// against r. Pass either a cookie OR a bearerToken (mutually
// exclusive); the helper handles header wiring + CSRF.
func doRequest(t *testing.T, r *gin.Engine, method, path string, body any, cookie *http.Cookie, csrfToken, bearerToken string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestCreateTokenViaCookieReturnsPlaintext exercises the happy path
// of POST /api/tokens — a cookie-authed call mints a PAT whose
// plaintext is surfaced in the response body.
func TestCreateTokenViaCookieReturnsPlaintext(t *testing.T) {
	r, sessionCookie, csrf := tokensTestRouter(t)

	rec := doRequest(t, r, "POST", "/api/tokens", map[string]any{
		"name":            "test-token",
		"scopes":          []string{"devices:read"},
		"expires_in_days": 0,
	}, sessionCookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp services.PATCreateResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" || resp.ID == "" {
		t.Fatalf("missing token/id in response: %+v", resp)
	}
}

// TestPATAuthCallsAllowedScope is the acceptance criterion from the
// plan: a PAT minted with devices:read scope can call GET /api/devices.
func TestPATAuthCallsAllowedScope(t *testing.T) {
	r, sessionCookie, csrf := tokensTestRouter(t)

	// Mint a devices:read PAT via the cookie session.
	rec := doRequest(t, r, "POST", "/api/tokens", map[string]any{
		"name":            "read-only",
		"scopes":          []string{"devices:read"},
		"expires_in_days": 0,
	}, sessionCookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("mint: %d %s", rec.Code, rec.Body.String())
	}
	var minted services.PATCreateResult
	_ = json.Unmarshal(rec.Body.Bytes(), &minted)

	// Now use the PAT bearer token on GET /api/devices — no cookie.
	rec = doRequest(t, r, "GET", "/api/devices", nil, nil, "", minted.Token)
	if rec.Code != http.StatusOK {
		t.Fatalf("PAT-authed GET /api/devices: status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

// TestPATAuthRejectedOnInsufficientScope is the other half of the
// plan's acceptance criterion: a devices:read PAT calling
// POST /api/bulk gets 403 scope-violation.
func TestPATAuthRejectedOnInsufficientScope(t *testing.T) {
	r, sessionCookie, csrf := tokensTestRouter(t)

	rec := doRequest(t, r, "POST", "/api/tokens", map[string]any{
		"name":            "read-only",
		"scopes":          []string{"devices:read"},
		"expires_in_days": 0,
	}, sessionCookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("mint: %d %s", rec.Code, rec.Body.String())
	}
	var minted services.PATCreateResult
	_ = json.Unmarshal(rec.Body.Bytes(), &minted)

	rec = doRequest(t, r, "POST", "/api/bulk", map[string]any{
		"action": "reboot",
		"macs":   []string{},
	}, nil, "", minted.Token)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("PAT-authed POST /api/bulk: status = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
	var errResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp["required_scope"] != "devices:write" {
		t.Errorf("403 body missing required_scope: %+v", errResp)
	}
}

// TestPATAuthSkipsCSRF locks in the CSRF-skip path: a PAT request can
// POST without a CSRF token (the bearer header IS the proof-of-intent).
func TestPATAuthSkipsCSRF(t *testing.T) {
	r, sessionCookie, csrf := tokensTestRouter(t)

	rec := doRequest(t, r, "POST", "/api/tokens", map[string]any{
		"name":            "writer",
		"scopes":          []string{"devices:write"},
		"expires_in_days": 0,
	}, sessionCookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("mint: %d %s", rec.Code, rec.Body.String())
	}
	var minted services.PATCreateResult
	_ = json.Unmarshal(rec.Body.Bytes(), &minted)

	// Same writer PAT calls POST /api/devices/refresh WITHOUT a CSRF
	// header — must NOT 403 on CSRF.
	rec = doRequest(t, r, "POST", "/api/devices/refresh", map[string]any{}, nil, "", minted.Token)
	// The handler itself may fail for reasons unrelated to auth (DB
	// state, job-spawn races), but it must NOT return 403 with
	// "invalid csrf token" body.
	if rec.Code == http.StatusForbidden {
		var errResp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
		if errResp["error"] == "invalid csrf token" {
			t.Fatalf("PAT-authed POST blocked by CSRF: body=%s", rec.Body.String())
		}
	}
}

// TestRevokedPATRejected locks in that a freshly-revoked token cannot
// be used on the next request.
func TestRevokedPATRejected(t *testing.T) {
	r, sessionCookie, csrf := tokensTestRouter(t)

	rec := doRequest(t, r, "POST", "/api/tokens", map[string]any{
		"name":            "soon-revoked",
		"scopes":          []string{"devices:read"},
		"expires_in_days": 0,
	}, sessionCookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("mint: %d %s", rec.Code, rec.Body.String())
	}
	var minted services.PATCreateResult
	_ = json.Unmarshal(rec.Body.Bytes(), &minted)

	// PAT works.
	rec = doRequest(t, r, "GET", "/api/devices", nil, nil, "", minted.Token)
	if rec.Code != http.StatusOK {
		t.Fatalf("pre-revoke GET: status = %d", rec.Code)
	}

	// Revoke via the cookie session.
	rec = doRequest(t, r, "DELETE", "/api/tokens/"+minted.ID, nil, sessionCookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// PAT must now be rejected.
	rec = doRequest(t, r, "GET", "/api/devices", nil, nil, "", minted.Token)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("post-revoke GET: status = %d, want 401, body=%s", rec.Code, rec.Body.String())
	}
}

// TestPATCannotMintAnotherPAT is the privilege-escalation guard: a
// PAT-authed caller hitting POST /api/tokens gets 403, even with the
// admin scope.
func TestPATCannotMintAnotherPAT(t *testing.T) {
	r, sessionCookie, csrf := tokensTestRouter(t)

	// Mint an admin-scoped PAT via cookie.
	rec := doRequest(t, r, "POST", "/api/tokens", map[string]any{
		"name":            "admin-pat",
		"scopes":          []string{"admin"},
		"expires_in_days": 0,
	}, sessionCookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("mint: %d %s", rec.Code, rec.Body.String())
	}
	var minted services.PATCreateResult
	_ = json.Unmarshal(rec.Body.Bytes(), &minted)

	// Use that PAT to try to mint another PAT. Must be refused.
	rec = doRequest(t, r, "POST", "/api/tokens", map[string]any{
		"name":            "second",
		"scopes":          []string{"admin"},
		"expires_in_days": 0,
	}, nil, "", minted.Token)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("PAT-authed mint: status = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
}

// TestInvalidBearerReturns401 locks in the failure-mode collapse: a
// malformed bearer token does not differentiate from a wrong one (both
// 401 with the same body shape).
func TestInvalidBearerReturns401(t *testing.T) {
	r, _, _ := tokensTestRouter(t)

	cases := []string{
		"not-a-pat",
		"pat_",
		"pat_aaaaaaaa_short",
	}
	for _, raw := range cases {
		rec := doRequest(t, r, "GET", "/api/devices", nil, nil, "", raw)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("bearer %q: status = %d, want 401, body=%s", raw, rec.Code, rec.Body.String())
		}
	}
}
