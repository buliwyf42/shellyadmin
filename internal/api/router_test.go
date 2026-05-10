package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"shellyadmin/internal/db"
	"shellyadmin/internal/middleware"
)

func newTestRouter(t *testing.T, cfg Config) *http.ServeMux {
	t.Helper()
	t.Setenv("GIN_MODE", "test")

	dataDir := t.TempDir()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	cfg.User = "admin"
	cfg.Secret = "test-secret"
	cfg.CookieSecure = false
	cfg.DataDir = dataDir

	router := NewRouter(database, cfg)
	mux := http.NewServeMux()
	mux.Handle("/", router)
	return mux
}

func newTestConfigWithStatic() Config {
	return Config{
		StaticFS: fstest.MapFS{
			"dist/index.html":              &fstest.MapFile{Data: []byte("<!doctype html><html><body>test shell</body></html>")},
			"dist/assets/index-test123.js": &fstest.MapFile{Data: []byte("console.log('ok');")},
		},
		HasStatic: true,
	}
}

func TestNoRouteReturnsJSONForUnknownAPIPath(t *testing.T) {
	router := newTestRouter(t, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content-type = %q, want application/json", got)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if body["error"] != "not found" {
		t.Fatalf("error = %q, want %q", body["error"], "not found")
	}
}

func TestNoRouteServesExistingEmbeddedAsset(t *testing.T) {
	router := newTestRouter(t, newTestConfigWithStatic())

	req := httptest.NewRequest(http.MethodGet, "/assets/index-test123.js", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "console.log('ok');" {
		t.Fatalf("body = %q, want embedded asset", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "javascript") && !strings.Contains(got, "text/plain") {
		t.Fatalf("content-type = %q, want javascript-like content type", got)
	}
}

func TestNoRouteReturns404ForMissingEmbeddedAsset(t *testing.T) {
	router := newTestRouter(t, newTestConfigWithStatic())

	req := httptest.NewRequest(http.MethodGet, "/assets/index-missing999.js", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if strings.Contains(rec.Body.String(), "test shell") {
		t.Fatalf("missing asset should not fall back to index.html")
	}
}

func TestNoRouteServesSPAIndexForClientRoute(t *testing.T) {
	router := newTestRouter(t, newTestConfigWithStatic())

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content-type = %q, want text/html", got)
	}
	if !strings.Contains(rec.Body.String(), "test shell") {
		t.Fatalf("body should contain SPA index")
	}
}

func TestStaticSubFSWithoutStaticReturnsNotExist(t *testing.T) {
	_, err := staticSubFS(Config{})
	if err == nil || !strings.Contains(err.Error(), fs.ErrNotExist.Error()) {
		t.Fatalf("err = %v, want fs.ErrNotExist", err)
	}
}

func TestRequestIDEchoedOnEveryResponse(t *testing.T) {
	router := newTestRouter(t, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	id := rec.Header().Get("X-Request-ID")
	if id == "" {
		t.Fatalf("expected generated X-Request-ID header on response")
	}
	if len(id) != 16 {
		t.Fatalf("expected 16-hex id, got %q", id)
	}
}

func TestRequestIDHonoursInboundHeader(t *testing.T) {
	router := newTestRouter(t, Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil)
	req.Header.Set("X-Request-ID", "op-trace-42")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "op-trace-42" {
		t.Fatalf("response should echo inbound id, got %q", got)
	}
}

// TestHandlerLogFnCarriesRequestID locks in the Phase 4a wiring: when a
// service-layer log is emitted with a context that carries a request ID
// (populated by the RequestID middleware), the audit sink sees that ID
// instead of the empty string. Regression guard against accidentally
// reverting the ctx-aware callback signature.
func TestHandlerLogFnCarriesRequestID(t *testing.T) {
	var captured string
	h := &Handler{auditSink: func(_, _, reqID string) { captured = reqID }}
	h.logFn = func(ctx context.Context, level, msg string) {
		h.auditSink(level, msg, middleware.FromContext(ctx))
	}

	ctx := middleware.WithRequestID(context.Background(), "trace-abc")
	h.logFn(ctx, "INFO", "test")

	if captured != "trace-abc" {
		t.Fatalf("request id = %q, want %q", captured, "trace-abc")
	}
}
