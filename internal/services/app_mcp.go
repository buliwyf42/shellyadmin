package services

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"shellyadmin/internal/db"
)

// MCPBuilder constructs an *http.Server fronting an MCP listener for the
// given token. Defaults to mcp.Build; tests inject a fake to avoid real
// network binds.
type MCPBuilder func(database *db.DB, dataDir, token, bind, port, version string) (*http.Server, error)

// MCPController encapsulates the live MCP-listener lifecycle on top of
// AppService. Keeping it separate from AppService's other state means
// the reconcile path doesn't have to take the service-wide mutex, and
// the test seam (the builder) stays isolated. AppService owns one
// instance; nil-checks in callers gate "MCP is wired up at all."
type MCPController struct {
	mu sync.Mutex

	// Set once at boot via SetMCPParams. envToken non-empty means
	// SHELLYADMIN_MCP_TOKEN was supplied — env wins, settings changes
	// are ignored.
	envToken string
	bind     string
	port     string
	version  string
	database *db.DB
	dataDir  string

	// Test seam — production code uses mcp.Build.
	builder MCPBuilder

	// Live state — protected by mu.
	server       *http.Server
	currentToken string
}

// SetMCPParams installs the runtime parameters the MCP listener needs to
// start. Called once from main.go after the DB is open and the service
// is constructed. The builder argument is required — passing it in (vs.
// importing mcp.Build directly here) avoids a services↔mcp import cycle
// since the mcp package itself imports services to expose tools backed
// by AppService methods. Tests pass a stub that returns an *http.Server
// bound to an in-memory listener / port 0.
//
// Idempotent — last call wins. envToken=="" means the listener will be
// settings-driven; non-empty means env-locked.
func (s *AppService) SetMCPParams(database *db.DB, builder MCPBuilder, envToken, bind, port, version string) {
	if s.mcp == nil {
		s.mcp = &MCPController{}
	}
	s.mcp.mu.Lock()
	defer s.mcp.mu.Unlock()
	s.mcp.envToken = envToken
	s.mcp.bind = bind
	s.mcp.port = port
	s.mcp.version = version
	s.mcp.database = database
	s.mcp.dataDir = s.dataDir
	s.mcp.builder = builder
}

// MCPManagedByEnv reports whether SHELLYADMIN_MCP_TOKEN was set at boot.
// The API GET handler uses this to populate the read-only flag the SPA
// renders the override notice from.
func (s *AppService) MCPManagedByEnv() bool {
	if s.mcp == nil {
		return false
	}
	s.mcp.mu.Lock()
	defer s.mcp.mu.Unlock()
	return s.mcp.envToken != ""
}

// MCPRunning reports whether an MCP listener goroutine is currently
// active. Surfaced in the API GET response so the SPA can render a
// live status badge in the MCP card.
func (s *AppService) MCPRunning() bool {
	if s.mcp == nil {
		return false
	}
	s.mcp.mu.Lock()
	defer s.mcp.mu.Unlock()
	return s.mcp.server != nil
}

// StartMCPFromConfig brings up MCP at boot. Resolution order matches
// the v0.1.20 design: env var first, settings second. Idempotent —
// reconcileMCP handles the no-change case.
func (s *AppService) StartMCPFromConfig() {
	if s.mcp == nil {
		return
	}
	s.mcp.mu.Lock()
	envToken := s.mcp.envToken
	s.mcp.mu.Unlock()

	token := envToken
	if token == "" {
		// Settings path. Read decrypted plaintext (GetSettings handles
		// the secretbox round-trip for internal callers).
		if persisted, err := s.GetSettings(); err == nil {
			if persisted.MCPEnabled && persisted.MCPToken != "" {
				token = persisted.MCPToken
				slog.Info("MCP enabled via settings (env var not set)")
			}
		} else {
			slog.Warn("MCP settings read failed; MCP disabled", "err", err)
		}
	}
	s.reconcileMCP(token)
}

// ReconcileMCPFromSettings is invoked by SaveSettings after the new
// settings have been persisted. It reads back what was just saved,
// decrypts the token, and starts/stops the listener to match. No-op
// when env-locked. Errors are logged but never propagated — settings
// save itself succeeded; an MCP startup failure should not roll back
// the operator's intent.
func (s *AppService) ReconcileMCPFromSettings() {
	if s.mcp == nil {
		return
	}
	s.mcp.mu.Lock()
	envLocked := s.mcp.envToken != ""
	s.mcp.mu.Unlock()
	if envLocked {
		return
	}
	persisted, err := s.GetSettings()
	if err != nil {
		slog.Warn("mcp reconcile: read settings failed", "err", err)
		return
	}
	token := ""
	if persisted.MCPEnabled && persisted.MCPToken != "" {
		token = persisted.MCPToken
	}
	s.reconcileMCP(token)
}

// stopMCP gracefully shuts the listener down. Called from Stop().
func (s *AppService) stopMCP(ctx context.Context) {
	if s.mcp == nil {
		return
	}
	s.mcp.mu.Lock()
	srv := s.mcp.server
	s.mcp.server = nil
	s.mcp.currentToken = ""
	s.mcp.mu.Unlock()
	if srv == nil {
		return
	}
	if err := srv.Shutdown(ctx); err != nil {
		slog.Warn("mcp shutdown", "err", err)
	}
}

// reconcileMCP is the single point that mutates the live listener.
// Called from StartMCPFromConfig, ReconcileMCPFromSettings, and tests.
// Holds c.mu for the entire lifecycle transition so concurrent saves
// serialize cleanly.
func (s *AppService) reconcileMCP(token string) {
	c := s.mcp
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if token == c.currentToken {
		// Already in the desired state. Includes the "stay disabled"
		// (token=="" → currentToken=="") and "no-op rotation" cases.
		return
	}

	// Tear down the existing listener (if any) before starting a new one.
	if c.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.server.Shutdown(ctx); err != nil {
			slog.Warn("mcp reconcile: shutdown old listener", "err", err)
		}
		cancel()
		slog.Info("MCP listener stopped")
		c.server = nil
		c.currentToken = ""
	}

	if token == "" {
		// Settings disabled MCP and we just stopped the listener — done.
		return
	}

	// Start the new listener.
	if c.database == nil || c.builder == nil {
		slog.Warn("mcp reconcile: not initialized (SetMCPParams not called)")
		return
	}
	srv, err := c.builder(c.database, c.dataDir, token, c.bind, c.port, c.version)
	if err != nil {
		slog.Error("mcp reconcile: build listener failed", "err", err)
		return
	}
	c.server = srv
	c.currentToken = token
	slog.Info("MCP server starting", "addr", srv.Addr)
	go func(server *http.Server) {
		if lerr := server.ListenAndServe(); lerr != nil && lerr != http.ErrServerClosed {
			slog.Error("mcp listen failed", "err", lerr)
		}
	}(srv)
}
