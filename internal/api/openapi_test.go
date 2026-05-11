package api

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/db"
)

func TestDocumentedAPIRoutesMatchExpectedRouteSet(t *testing.T) {
	got := make([]string, 0, len(documentedAPIRoutes()))
	for _, route := range documentedAPIRoutes() {
		got = append(got, route.Method+" "+route.Path)
	}
	sort.Strings(got)

	want := []string{
		http.MethodDelete + " /api/credential-groups/{name}",
		http.MethodDelete + " /api/credentials/{name}",
		http.MethodDelete + " /api/logs",
		http.MethodDelete + " /api/templates/{name}",
		http.MethodGet + " /api/csrf-token",
		http.MethodGet + " /api/backup/export",
		http.MethodGet + " /api/credential-groups",
		http.MethodGet + " /api/credential-groups/assignments",
		http.MethodGet + " /api/credentials",
		http.MethodGet + " /api/devices",
		http.MethodGet + " /api/devices/{target}",
		http.MethodGet + " /api/devices/{target}/actions",
		http.MethodGet + " /api/devices/{target}/export",
		http.MethodGet + " /api/firmware/install/status",
		http.MethodGet + " /api/firmware/status",
		http.MethodGet + " /api/logs",
		http.MethodGet + " /api/logs/export",
		http.MethodGet + " /api/openapi/v1.json",
		http.MethodGet + " /api/scan/status",
		http.MethodGet + " /api/settings",
		http.MethodGet + " /api/templates",
		http.MethodGet + " /api/templates/{name}",
		http.MethodGet + " /api/totp/status",
		http.MethodGet + " /api/version",
		http.MethodGet + " /health",
		http.MethodGet + " /ready",
		http.MethodPost + " /api/bulk",
		http.MethodPost + " /api/backup/import",
		http.MethodPost + " /api/credential-groups",
		http.MethodPost + " /api/credential-groups/assignments",
		http.MethodPost + " /api/credentials",
		http.MethodPost + " /api/devices/forget",
		http.MethodPost + " /api/devices/refresh",
		http.MethodPost + " /api/devices/refresh-one",
		http.MethodPost + " /api/devices/{target}/actions/{action}",
		http.MethodPost + " /api/firmware/check",
		http.MethodPost + " /api/firmware/update",
		http.MethodPost + " /api/login",
		http.MethodPost + " /api/logout",
		http.MethodPost + " /api/provision",
		http.MethodPost + " /api/provision/user-ca",
		http.MethodPost + " /api/scan/confirm",
		http.MethodPost + " /api/scan/start",
		http.MethodPost + " /api/settings",
		http.MethodPost + " /api/templates/{name}",
		http.MethodPost + " /api/totp/disable",
		http.MethodPost + " /api/totp/enroll",
		http.MethodPost + " /api/totp/verify-enroll",
	}
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("route count = %d, want %d\nroutes=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("route[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestOpenAPIV1SpecIncludesEveryDocumentedRoute(t *testing.T) {
	spec := openAPIV1Spec()
	paths, ok := spec["paths"].(gin.H)
	if !ok {
		t.Fatalf("paths type = %T, want gin.H", spec["paths"])
	}

	for _, route := range documentedAPIRoutes() {
		pathItem, ok := paths[route.Path].(gin.H)
		if !ok {
			t.Fatalf("missing path item for %s", route.Path)
		}
		operation, ok := pathItem[strings.ToLower(route.Method)].(gin.H)
		if !ok {
			t.Fatalf("missing operation for %s %s", route.Method, route.Path)
		}
		if operation["summary"] != route.Summary {
			t.Fatalf("summary for %s %s = %v, want %q", route.Method, route.Path, operation["summary"], route.Summary)
		}
	}
}

// TestEveryAPIRouteIsDocumented locks in M9: every route the live gin
// router serves under `/api/`, `/health`, or `/ready` must appear in
// `documentedAPIRoutes()`. The reverse direction (every documented
// route is registered) is covered by the route-set test above; together
// they guarantee the OpenAPI spec, the registered routes, and the test
// allowlist stay in sync. Adding a route without documenting it now
// fails CI rather than silently shipping an undocumented surface.
func TestEveryAPIRouteIsDocumented(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dataDir := t.TempDir()
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	engine := NewRouter(database, Config{
		User:    "admin",
		Secret:  "test-secret",
		DataDir: dataDir,
	})

	documented := make(map[string]bool)
	for _, r := range documentedAPIRoutes() {
		// gin renders path params as `:name` internally; convert
		// the OpenAPI `{name}` form to match.
		key := r.Method + " " + openAPIToGinPath(r.Path)
		documented[key] = true
	}

	for _, r := range engine.Routes() {
		path := r.Path
		// Skip routes outside the documented surface (SPA fallthrough,
		// static assets, login GET, app-shell routes for client-side
		// navigation). These are intentionally NOT API endpoints.
		if !strings.HasPrefix(path, "/api/") && path != "/health" && path != "/ready" {
			continue
		}
		// The /api/logout path is registered as a top-level POST, not
		// inside the auth group's documented set with the same prefix —
		// already in documentedAPIRoutes, just normalize.
		key := r.Method + " " + path
		if !documented[key] {
			t.Errorf("undocumented route: %s %s — add it to documentedAPIRoutes() in internal/api/openapi.go", r.Method, path)
		}
	}
}

// openAPIToGinPath converts `/api/{name}` → `/api/:name` so the gin
// router output matches the OpenAPI path templates.
func openAPIToGinPath(p string) string {
	// Cheap two-pass replace; both sides are static so no need for regex.
	for strings.Contains(p, "{") {
		open := strings.Index(p, "{")
		close := strings.Index(p, "}")
		if close <= open {
			break
		}
		p = p[:open] + ":" + p[open+1:close] + p[close+1:]
	}
	return p
}
