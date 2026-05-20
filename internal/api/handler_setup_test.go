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

// setupTestRouter builds a full router with a fresh DB and NO env password
// hash, so the configured-state is driven purely by the admin_credentials
// table (the first-run setup path).
func setupTestRouter(t *testing.T) *gin.Engine {
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

	cfg := Config{
		User:    "admin",
		Secret:  "test-secret",
		DataDir: dataDir,
	}
	cfg.Service = services.NewAppService(database, dataDir, func(context.Context, string, string) {})
	return NewRouter(database, cfg)
}

func getSetupStatus(t *testing.T, r *gin.Engine) bool {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/setup/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp SetupStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode setup status: %v", err)
	}
	return resp.Configured
}

func postSetup(t *testing.T, r *gin.Engine, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest("POST", "/api/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestSetupFlow walks the full first-run path: unconfigured → setup → login.
func TestSetupFlow(t *testing.T) {
	r := setupTestRouter(t)

	if getSetupStatus(t, r) {
		t.Fatalf("expected configured=false on a fresh instance")
	}

	if rec := postSetup(t, r, "operator", "hunter2hunter2"); rec.Code != http.StatusOK {
		t.Fatalf("setup status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	if !getSetupStatus(t, r) {
		t.Fatalf("expected configured=true after setup")
	}

	// The freshly-created account must be able to log in.
	if rec := postLogin(t, r, "operator", "hunter2hunter2"); rec.Code != http.StatusOK {
		t.Fatalf("login after setup = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

// TestSetupRejectsSecondAttempt locks in the one-shot contract: once an
// account exists, a second setup POST must not overwrite it.
func TestSetupRejectsSecondAttempt(t *testing.T) {
	r := setupTestRouter(t)
	if rec := postSetup(t, r, "operator", "hunter2hunter2"); rec.Code != http.StatusOK {
		t.Fatalf("first setup = %d, want 200", rec.Code)
	}
	if rec := postSetup(t, r, "attacker", "differentpass"); rec.Code != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409, body=%s", rec.Code, rec.Body.String())
	}
	// The original credential still works; the second attempt did not win.
	if rec := postLogin(t, r, "operator", "hunter2hunter2"); rec.Code != http.StatusOK {
		t.Fatalf("original login = %d, want 200", rec.Code)
	}
	if rec := postLogin(t, r, "attacker", "differentpass"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("overwrite login = %d, want 401", rec.Code)
	}
}

// TestSetupRejectsShortPassword guards the usability floor.
func TestSetupRejectsShortPassword(t *testing.T) {
	r := setupTestRouter(t)
	if rec := postSetup(t, r, "operator", "short"); rec.Code != http.StatusBadRequest {
		t.Fatalf("short-password setup = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	if getSetupStatus(t, r) {
		t.Fatalf("expected still-unconfigured after a rejected setup")
	}
}
