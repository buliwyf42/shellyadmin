package api

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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
		http.MethodGet + " /api/firmware/status",
		http.MethodGet + " /api/logs",
		http.MethodGet + " /api/logs/export",
		http.MethodGet + " /api/openapi/v1.json",
		http.MethodGet + " /api/scan/status",
		http.MethodGet + " /api/settings",
		http.MethodGet + " /api/templates",
		http.MethodGet + " /api/templates/{name}",
		http.MethodGet + " /api/version",
		http.MethodGet + " /health",
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
		http.MethodPost + " /api/scan/confirm",
		http.MethodPost + " /api/scan/start",
		http.MethodPost + " /api/settings",
		http.MethodPost + " /api/templates/{name}",
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
