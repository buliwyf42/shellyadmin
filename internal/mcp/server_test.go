package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"shellyadmin/internal/db"
	"shellyadmin/internal/middleware"
	"shellyadmin/internal/models"
	"shellyadmin/internal/services"
)

func newTestEnv(t *testing.T) (*db.DB, *services.AppService, *http.Server) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	srv, err := Build(database, t.TempDir(), "test-token", "127.0.0.1", "0", "v-test")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	svc := services.NewAppService(database, t.TempDir(), func(context.Context, string, string) {})
	return database, svc, srv
}

func TestBuildRejectsEmptyToken(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := Build(database, t.TempDir(), "", "127.0.0.1", "8081", "v-test"); err == nil {
		t.Fatalf("Build with empty token should error")
	}
}

func TestHTTPMissingAuthReturns401(t *testing.T) {
	_, _, srv := newTestEnv(t)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Post(ts.URL, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestHTTPWrongAuthReturns401(t *testing.T) {
	_, _, srv := newTestEnv(t)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer not-the-right-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

// bearerTransport adds the Authorization header to every outbound MCP
// request so the SDK client can authenticate against the auth middleware.
type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (b *bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

// TestEndToEndListAndCallTool spins up the full handler chain and connects
// a real MCP client to it with the correct bearer token. Confirms that
// tools/list returns the registered tools and that calling list_devices
// returns the seeded device.
func TestEndToEndListAndCallTool(t *testing.T) {
	database, _, srv := newTestEnv(t)
	if err := database.UpsertDevice(models.Device{
		MAC:    "AA:BB:CC:DD:EE:01",
		IP:     "192.168.1.10",
		Name:   "kitchen-plug",
		App:    "PlugSG3",
		Gen:    3,
		Online: true,
	}); err != nil {
		t.Fatalf("seed device: %v", err)
	}

	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	transport := &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: &bearerTransport{token: "test-token", base: http.DefaultTransport}},
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "shellyadmin-test", Version: "v0"}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	wantNames := []string{
		"list_devices", "get_device", "list_device_actions", "scan_status",
		"firmware_status", "firmware_install_status", "list_templates",
		"get_template", "list_credentials", "get_settings", "get_logs",
		"export_device", "compliance_summary",
	}
	got := map[string]bool{}
	for _, t := range tools.Tools {
		got[t.Name] = true
	}
	for _, name := range wantNames {
		if !got[name] {
			t.Errorf("ListTools missing %q (got %d tools: %v)", name, len(tools.Tools), got)
		}
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_devices",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool list_devices: %v", err)
	}
	if res.IsError {
		t.Fatalf("list_devices returned tool error: %+v", res)
	}
	// Structured content should carry the seeded device. The SDK exposes
	// the typed Out value through StructuredContent.
	body := ""
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			body += tc.Text
		}
	}
	if !strings.Contains(body, "kitchen-plug") {
		t.Errorf("list_devices output did not include seeded device; body=%s", body)
	}
}

// TestRequestIDMiddlewareEchoesHeader verifies the middleware honors a
// client-supplied X-Request-ID and propagates it via context.
func TestRequestIDMiddlewareEchoesHeader(t *testing.T) {
	var seen string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = middleware.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := requestIDMiddleware(inner)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(middleware.HeaderRequestID, "abc-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if seen != "abc-123" {
		t.Errorf("context request id = %q, want %q", seen, "abc-123")
	}
	if got := rec.Header().Get(middleware.HeaderRequestID); got != "abc-123" {
		t.Errorf("response header request id = %q, want %q", got, "abc-123")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if got := rec2.Header().Get(middleware.HeaderRequestID); len(got) != 16 {
		t.Errorf("auto-generated id = %q (len %d), want 16-char hex", got, len(got))
	}
}
