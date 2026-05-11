package services

// Delegators to internal/services/mcp. The MCP listener lifecycle moved to
// its own sub-package in v0.3.0 (M7, docs/plans/phase-4b-refactor-block.md
// Block 4b.1); these AppService methods preserve the public surface
// (cmd/shellyctl/main.go, api/handler_settings.go, api/handler_meta.go,
// internal/services/app_mcp_test.go) so call sites compile unchanged.

import (
	"context"
	"log/slog"

	"shellyadmin/internal/db"
	mcpctl "shellyadmin/internal/services/mcp"
)

// MCPBuilder constructs an *http.Server fronting an MCP listener for the
// given token. Re-export of mcpctl.Builder so cmd/shellyctl/main.go (and the
// existing app_mcp_test.go) doesn't need to import the sub-package.
type MCPBuilder = mcpctl.Builder

// MCPController is re-exported so existing callers that named the type keep
// compiling. New code in this package should reach for *mcpctl.Controller.
type MCPController = mcpctl.Controller

// SetMCPParams installs the runtime parameters the MCP listener needs to
// start. Called once from main.go after the DB is open and the service is
// constructed. The builder argument is required — passing it in (vs.
// importing mcp.Build directly here) avoids a services↔mcp import cycle.
// Idempotent — last call wins. envToken=="" means the listener will be
// settings-driven; non-empty means env-locked.
func (s *AppService) SetMCPParams(database *db.DB, builder MCPBuilder, envToken, bind, port, version string) {
	if s.mcp == nil {
		s.mcp = mcpctl.New()
	}
	s.mcp.SetParams(database, builder, envToken, bind, port, version, s.dataDir)
}

// MCPManagedByEnv reports whether SHELLYADMIN_MCP_TOKEN was set at boot.
// The API GET handler uses this to populate the read-only flag the SPA
// renders the override notice from.
func (s *AppService) MCPManagedByEnv() bool {
	return s.mcp.EnvLocked()
}

// MCPRunning reports whether an MCP listener goroutine is currently active.
// Surfaced in the API GET response so the SPA can render a live status badge.
func (s *AppService) MCPRunning() bool {
	return s.mcp.Running()
}

// StartMCPFromConfig brings up MCP at boot. Resolution order matches the
// v0.1.20 design: env var first, settings second. Idempotent — Reconcile
// handles the no-change case.
func (s *AppService) StartMCPFromConfig() {
	if s.mcp == nil {
		return
	}
	token := s.mcp.EnvToken()
	if token == "" {
		// Settings path. GetSettings decrypts the persisted secretbox
		// envelope so internal callers see plaintext.
		if persisted, err := s.GetSettings(); err == nil {
			if persisted.MCPEnabled && persisted.MCPToken != "" {
				token = persisted.MCPToken
				slog.Info("MCP enabled via settings (env var not set)")
			}
		} else {
			slog.Warn("MCP settings read failed; MCP disabled", "err", err)
		}
	}
	s.mcp.Reconcile(token)
}

// ReconcileMCPFromSettings is invoked by SaveSettings after the new settings
// have been persisted. It reads back what was just saved, decrypts the
// token, and starts/stops the listener to match. No-op when env-locked.
// Errors are logged but never propagated — settings save itself succeeded;
// an MCP startup failure should not roll back the operator's intent.
func (s *AppService) ReconcileMCPFromSettings() {
	if s.mcp == nil || s.mcp.EnvLocked() {
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
	s.mcp.Reconcile(token)
}

// stopMCP gracefully shuts the listener down. Called from Stop().
func (s *AppService) stopMCP(ctx context.Context) {
	s.mcp.Stop(ctx)
}
