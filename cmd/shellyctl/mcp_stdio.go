package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"shellyadmin/internal/db"
	"shellyadmin/internal/mcp"
	"shellyadmin/internal/services"
)

// runMCPStdio is the entry point for `shellyctl mcp` — exposes the MCP
// tool surface over stdin/stdout for stdio MCP clients (Claude Desktop's
// MCP config block, mcp-remote, etc.) without binding the HTTP listener.
//
// The HTTP MCP server is the long-lived, remote-access path; stdio is
// for "Claude Desktop on the same host" workflows where the parent
// process spawning the binary IS the trust boundary (no transport
// auth). Both paths share the same tools via internal/mcp.register.
//
// Logs go to stderr — stdout carries JSON-RPC frames the client parses.
// Background workers (firmware-check scheduler etc.) are NOT started:
// this is a query session, not a long-running server, and the workers
// would race with a parallel container holding the same data dir.
func runMCPStdio() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	dataDir := getenv("DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mcp stdio: create data dir: %v\n", err)
		os.Exit(1)
	}
	if err := loadEncryptionKey(dataDir, slog.Default()); err != nil {
		fmt.Fprintf(os.Stderr, "mcp stdio: encryption key init: %v\n", err)
		os.Exit(1)
	}

	database, err := db.Open(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mcp stdio: db open: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	svc := services.NewAppService(database, dataDir, func(_ context.Context, level, msg string) {
		_ = database.AddLog(level, services.SanitizeLogMessage(msg))
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := mcp.RunStdio(ctx, svc, resolveBackendVersion()); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "mcp stdio: server exited: %v\n", err)
		os.Exit(1)
	}
}
