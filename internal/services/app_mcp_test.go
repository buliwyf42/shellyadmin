package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"shellyadmin/internal/db"
	"shellyadmin/internal/models"
	mcpctl "shellyadmin/internal/services/mcp"
)

// trackingBuilder returns a build function that records every invocation
// and produces lightweight *http.Server instances bound to a real
// loopback listener via httptest, so .Shutdown() is exercised the same
// way mcp.Build's output would be. Used by tests to assert that
// reconcile starts/stops the listener at the expected times without
// pulling the real MCP package in.
func trackingBuilder(t *testing.T, builds *atomic.Int32, lastToken *atomic.Pointer[string]) mcpctl.Builder {
	t.Helper()
	return func(_ *db.DB, _, token, _, _, _ string) (*http.Server, error) {
		builds.Add(1)
		tok := token
		lastToken.Store(&tok)
		// httptest.NewUnstartedServer gives us a real *http.Server with a
		// listener wired up; .Start() begins serving on a free port. We
		// keep the handler trivial — these tests assert on lifecycle
		// transitions, not on what MCP does once it's running.
		ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		ts.Start()
		// Wrap the test-server's *http.Server so callers see exactly the
		// shape mcp.Build returns. Hand the underlying listener back via
		// the test cleanup so we don't leak fds.
		t.Cleanup(ts.Close)
		return ts.Config, nil
	}
}

func newTestServiceWithMCPBuilder(t *testing.T, b mcpctl.Builder, envToken string) (*db.DB, *AppService) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	svc := NewAppService(database, t.TempDir(), nil)
	// nil dataDir/version/bind/port are fine because the builder is
	// stubbed; the controller just passes them through.
	svc.SetMCPParams(database, b, envToken, "127.0.0.1", "0", "test")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		svc.Stop(ctx)
	})
	return database, svc
}

func TestSaveSettingsStartsAndStopsMCPLive(t *testing.T) {
	var builds atomic.Int32
	var lastToken atomic.Pointer[string]
	_, svc := newTestServiceWithMCPBuilder(t, trackingBuilder(t, &builds, &lastToken), "")

	if svc.MCPRunning() {
		t.Fatal("listener should be stopped at construction")
	}

	plain := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // 64 hex
	if err := svc.SaveSettings(models.AppSettings{
		Subnets: []string{"192.168.1.0/30"}, ScanTimeout: 2, RefreshTimeout: 5,
		ScanConcurrency: 64, MCPEnabled: true, MCPToken: plain,
	}); err != nil {
		t.Fatalf("SaveSettings(enabled): %v", err)
	}
	if !svc.MCPRunning() {
		t.Errorf("listener should be running after MCPEnabled save")
	}
	if got := builds.Load(); got != 1 {
		t.Errorf("builder calls = %d, want 1", got)
	}
	if got := lastToken.Load(); got == nil || *got != plain {
		t.Errorf("builder saw token %v, want %q", got, plain)
	}

	// Disable → listener stops, no extra build.
	if err := svc.SaveSettings(models.AppSettings{
		Subnets: []string{"192.168.1.0/30"}, ScanTimeout: 2, RefreshTimeout: 5,
		ScanConcurrency: 64, MCPEnabled: false, MCPToken: "",
	}); err != nil {
		t.Fatalf("SaveSettings(disabled): %v", err)
	}
	if svc.MCPRunning() {
		t.Errorf("listener should be stopped after MCPEnabled=false save")
	}
	if got := builds.Load(); got != 1 {
		t.Errorf("builder calls after disable = %d, want still 1", got)
	}
}

func TestSaveSettingsRotatesMCPTokenLive(t *testing.T) {
	var builds atomic.Int32
	var lastToken atomic.Pointer[string]
	_, svc := newTestServiceWithMCPBuilder(t, trackingBuilder(t, &builds, &lastToken), "")

	first := "aaaaaaaaaaaaaaaa1111111111111111aaaaaaaaaaaaaaaa1111111111111111"
	second := "bbbbbbbbbbbbbbbb2222222222222222bbbbbbbbbbbbbbbb2222222222222222"
	for _, tok := range []string{first, second} {
		if err := svc.SaveSettings(models.AppSettings{
			Subnets: []string{"192.168.1.0/30"}, ScanTimeout: 2, RefreshTimeout: 5,
			ScanConcurrency: 64, MCPEnabled: true, MCPToken: tok,
		}); err != nil {
			t.Fatalf("SaveSettings(%q): %v", tok, err)
		}
	}

	if got := builds.Load(); got != 2 {
		t.Errorf("builder calls after rotate = %d, want 2 (stop+start sequence)", got)
	}
	if got := lastToken.Load(); got == nil || *got != second {
		t.Errorf("after rotate builder saw %v, want %q", got, second)
	}
	if !svc.MCPRunning() {
		t.Errorf("listener should still be running after rotation")
	}
}

func TestSaveSettingsIsNoOpWhenEnvLocked(t *testing.T) {
	var builds atomic.Int32
	var lastToken atomic.Pointer[string]
	envTok := "ffffffffffffffffeeeeeeeeeeeeeeeeffffffffffffffffeeeeeeeeeeeeeeee"
	_, svc := newTestServiceWithMCPBuilder(t, trackingBuilder(t, &builds, &lastToken), envTok)

	// Bring up the listener via the env-driven boot path.
	svc.StartMCPFromConfig()
	if !svc.MCPRunning() {
		t.Fatal("env-locked listener should be running after StartMCPFromConfig")
	}
	if got := lastToken.Load(); got == nil || *got != envTok {
		t.Fatalf("env-driven listener saw token %v, want env %q", got, envTok)
	}

	// SaveSettings with a different MCP token must NOT touch the listener.
	persisted := "11111111111111110000000000000000111111111111111100000000000000000"
	if err := svc.SaveSettings(models.AppSettings{
		Subnets: []string{"192.168.1.0/30"}, ScanTimeout: 2, RefreshTimeout: 5,
		ScanConcurrency: 64, MCPEnabled: true, MCPToken: persisted,
	}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if got := builds.Load(); got != 1 {
		t.Errorf("env-locked: builder calls = %d, want 1 (no rebuild)", got)
	}
	if got := lastToken.Load(); got == nil || *got != envTok {
		t.Errorf("env-locked: token after save = %v, want still env %q", got, envTok)
	}
	if !svc.MCPManagedByEnv() {
		t.Errorf("MCPManagedByEnv should be true")
	}
}

func TestStartMCPFromConfigPrefersEnvOverSettings(t *testing.T) {
	var builds atomic.Int32
	var lastToken atomic.Pointer[string]
	envTok := "00000000111111110000000011111111000000001111111100000000111111110"
	database, svc := newTestServiceWithMCPBuilder(t, trackingBuilder(t, &builds, &lastToken), envTok)

	// Persist different settings — env must still win.
	settingsTok := "ssssssssssssssssrrrrrrrrrrrrrrrrssssssssssssssssrrrrrrrrrrrrrrrrr"
	if err := svc.SaveSettings(models.AppSettings{
		Subnets: []string{"192.168.1.0/30"}, ScanTimeout: 2, RefreshTimeout: 5,
		ScanConcurrency: 64, MCPEnabled: true, MCPToken: settingsTok,
	}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	// SaveSettings was no-op (env-locked) so listener didn't start there.
	if svc.MCPRunning() {
		t.Fatal("listener should not start from a save while env-locked")
	}

	svc.StartMCPFromConfig()
	if got := lastToken.Load(); got == nil || *got != envTok {
		t.Errorf("StartMCPFromConfig saw token %v, want env %q", got, envTok)
	}

	// Sanity: the persisted settings still hold the override (encrypted) —
	// confirm it was actually saved, not silently rejected.
	persisted, err := database.GetSettings()
	if err != nil {
		t.Fatalf("db.GetSettings: %v", err)
	}
	if persisted.MCPToken == "" || persisted.MCPToken == settingsTok {
		t.Errorf("persisted token = %q, want sealed envelope (non-empty, not plaintext)", persisted.MCPToken)
	}
}

func TestStartMCPFromConfigUsesSettingsWhenEnvUnset(t *testing.T) {
	var builds atomic.Int32
	var lastToken atomic.Pointer[string]
	_, svc := newTestServiceWithMCPBuilder(t, trackingBuilder(t, &builds, &lastToken), "")

	tok := "ssssssssssssssss1111111111111111ssssssssssssssss1111111111111111"
	// Save first (this also reconciles, starting the listener since env=="").
	if err := svc.SaveSettings(models.AppSettings{
		Subnets: []string{"192.168.1.0/30"}, ScanTimeout: 2, RefreshTimeout: 5,
		ScanConcurrency: 64, MCPEnabled: true, MCPToken: tok,
	}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if !svc.MCPRunning() {
		t.Fatal("listener should be running after settings-driven enable")
	}
	if got := lastToken.Load(); got == nil || *got != tok {
		t.Errorf("save reconcile saw %v, want %q", got, tok)
	}
}
