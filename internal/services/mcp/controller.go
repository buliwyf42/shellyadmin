// Package mcp owns the live MCP-listener lifecycle on top of *AppService.
// Keeping it in its own package means the reconcile path doesn't have to
// take the service-wide mutex, and the test seam (Builder) stays isolated.
//
// MOVED FROM internal/services/app_mcp.go — v0.3.0 services-layer split
// (M7, docs/plans/phase-4b-refactor-block.md Block 4b.1). AppService keeps a
// single *Controller and delegates the public surface (SetMCPParams,
// StartMCPFromConfig, ReconcileMCPFromSettings, MCPManagedByEnv, MCPRunning)
// to it.
package mcp

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"shellyadmin/internal/db"
)

// Builder constructs an *http.Server fronting an MCP listener for the given
// token. Defaults to internal/mcp.Build at the wiring site; tests inject a
// fake to avoid real network binds.
type Builder func(database *db.DB, dataDir, token, bind, port, version string) (*http.Server, error)

// Controller encapsulates the live MCP-listener lifecycle. AppService owns
// one instance; nil-checks in callers gate "MCP is wired up at all".
type Controller struct {
	mu sync.Mutex

	// Set once at boot via SetParams. envToken non-empty means
	// SHELLYADMIN_MCP_TOKEN was supplied — env wins, settings changes
	// are ignored.
	envToken string
	bind     string
	port     string
	version  string
	database *db.DB
	dataDir  string

	// Test seam — production code passes internal/mcp.Build.
	builder Builder

	// Live state — protected by mu.
	server       *http.Server
	currentToken string
}

// New constructs an empty Controller. The Controller is inert until SetParams
// is called; Reconcile is a no-op when database or builder is nil.
func New() *Controller { return &Controller{} }

// SetParams installs the runtime parameters the MCP listener needs to start.
// Called once from main.go after the DB is open and the service is
// constructed. Idempotent — last call wins. envToken=="" means the listener
// will be settings-driven; non-empty means env-locked.
func (c *Controller) SetParams(database *db.DB, builder Builder, envToken, bind, port, version, dataDir string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.envToken = envToken
	c.bind = bind
	c.port = port
	c.version = version
	c.database = database
	c.dataDir = dataDir
	c.builder = builder
}

// EnvToken returns the SHELLYADMIN_MCP_TOKEN value supplied at boot (or "")
// for AppService.StartMCPFromConfig to decide which source wins.
func (c *Controller) EnvToken() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.envToken
}

// EnvLocked reports whether SHELLYADMIN_MCP_TOKEN was set at boot. While
// env-locked, ReconcileFromSettings is a no-op.
func (c *Controller) EnvLocked() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.envToken != ""
}

// Running reports whether an MCP listener goroutine is currently active.
// Surfaced in the API GET response so the SPA can render a live status badge.
func (c *Controller) Running() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.server != nil
}

// Stop gracefully shuts the listener down. Called from AppService.Stop.
func (c *Controller) Stop(ctx context.Context) {
	if c == nil {
		return
	}
	c.mu.Lock()
	srv := c.server
	c.server = nil
	c.currentToken = ""
	c.mu.Unlock()
	if srv == nil {
		return
	}
	if err := srv.Shutdown(ctx); err != nil {
		slog.Warn("mcp shutdown", "err", err)
	}
}

// Reconcile is the single point that mutates the live listener. Holds the
// controller mutex for the entire lifecycle transition so concurrent saves
// serialize cleanly. An empty token tears down any active listener; a
// non-empty token (re)starts it.
func (c *Controller) Reconcile(token string) {
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
		slog.Warn("mcp reconcile: not initialized (SetParams not called)")
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
