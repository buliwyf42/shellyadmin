package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"shellyadmin/internal/services"
)

// RunStdio builds the MCP server with all tools registered, then serves
// it on stdin/stdout via the SDK's StdioTransport. Blocks until ctx is
// cancelled or the client disconnects.
//
// The caller is responsible for:
//   - constructing svc against the database it owns;
//   - routing logs to stderr (NOT stdout — stdout carries the JSON-RPC
//     frames the MCP client parses);
//   - NOT starting AppService background workers (this is a query
//     session, not a long-running server — the workers would race with
//     a parallel long-running container holding the same data dir).
//
// version is the resolved app version stamped on the MCP Implementation
// announce so clients can record which build they're talking to.
//
// Stdio mode has no transport-level auth — the parent process spawning
// the binary IS the trust boundary. Operators wire this into Claude
// Desktop's MCP config block; the host filesystem permissions on the
// data directory are the remaining gate.
func RunStdio(ctx context.Context, svc *services.AppService, version string) error {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "shellyadmin",
		Version: version,
	}, nil)
	register(server, svc)
	return server.Run(ctx, &mcp.StdioTransport{})
}
