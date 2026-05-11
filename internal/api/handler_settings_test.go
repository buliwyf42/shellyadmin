package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/core/secretbox"
	"shellyadmin/internal/db"
	"shellyadmin/internal/models"
	"shellyadmin/internal/services"
)

// ensureSecretboxKey installs a deterministic test key once per process so
// SaveSettings can seal the MCP token. The key value is fixed for tests —
// production loads it from SHELLYADMIN_ENCRYPTION_KEY / _FILE.
func ensureSecretboxKey(t *testing.T) {
	t.Helper()
	if secretbox.HasKey() {
		return
	}
	k, err := secretbox.GenerateKey()
	if err != nil {
		t.Fatalf("secretbox.GenerateKey() error = %v", err)
	}
	if err := secretbox.SetKey(k); err != nil {
		t.Fatalf("secretbox.SetKey() error = %v", err)
	}
}

// TestGetSettingsRedactsMCPToken locks in the contract that the SPA never
// sees plaintext MCP tokens. Regression guard against a future refactor that
// forgets to apply MCPTokenRedacted before serialising — a missing redaction
// would leak the token over /api/settings to anyone with an admin cookie
// and to the browser DevTools / extensions / cached responses.
func TestGetSettingsRedactsMCPToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureSecretboxKey(t)

	dataDir := t.TempDir()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	h := NewHandler(database, Config{DataDir: dataDir})

	tokenPlain := "abcdef0123456789-this-is-a-real-token-value"
	settings := defaultSettingsWithScanRange()
	settings.MCPEnabled = true
	settings.MCPToken = tokenPlain
	if err := h.service.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings() error = %v", err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "/api/settings", nil)
	h.GetSettings(c)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got models.AppSettings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, body=%s", err, rec.Body.String())
	}
	if got.MCPToken != services.MCPTokenRedacted {
		t.Fatalf("MCPToken = %q, want %q (redaction failed)", got.MCPToken, services.MCPTokenRedacted)
	}
	if strings.Contains(rec.Body.String(), tokenPlain) {
		t.Fatalf("response body contains plaintext token %q — redaction is broken", tokenPlain)
	}
}

// TestSaveSettingsRejectsInvalidMCPTokenFormat verifies the URL-safe alphabet
// constraint added in Phase 1. A token containing "/" would break MCP path
// auth ("/token/rpc" splits on "/"); a token shorter than 16 chars is
// trivially brute-forceable.
func TestSaveSettingsRejectsInvalidMCPTokenFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dataDir := t.TempDir()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	h := NewHandler(database, Config{DataDir: dataDir})

	cases := []struct {
		name  string
		token string
	}{
		{"contains-slash", "abcdef0123456789/bad"},
		{"contains-space", "abcdef 0123456789---"},
		{"too-short", "abc123"},
		{"contains-hash", "abcdef0123456789#nope"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := defaultSettingsWithScanRange()
			settings.MCPEnabled = true
			settings.MCPToken = tc.token
			err := h.service.SaveSettings(settings)
			if err == nil {
				t.Fatalf("SaveSettings(%q) returned nil, want validation error", tc.token)
			}
		})
	}
}

// defaultSettingsWithScanRange satisfies ValidateSettings' "at least one
// scan target" rule so we can exercise SaveSettings without bringing up
// every UI knob the tests don't actually care about.
func defaultSettingsWithScanRange() models.AppSettings {
	s := models.DefaultSettings()
	s.Subnets = []string{"192.168.1.0/24"}
	return s
}
