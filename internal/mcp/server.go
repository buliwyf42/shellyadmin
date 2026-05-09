package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"shellyadmin/internal/db"
	"shellyadmin/internal/middleware"
	"shellyadmin/internal/services"
)

// Build constructs the MCP HTTP listener wrapping a freshly built
// *services.AppService over the shared *db.DB. The returned *http.Server
// has the same timeouts as the main API listener and is owned by the
// caller; cmd/shellyctl/main.go runs it in a goroutine and Shutdown()s
// it during signal handling.
//
// token must be non-empty — Build returns an error otherwise. Callers
// gate construction on the operator having set SHELLYADMIN_MCP_TOKEN.
//
// version is the resolved app version stamped on the MCP Implementation
// announce message so clients can record which build they're talking to.
func Build(database *db.DB, dataDir, token, bind, port, version string) (*http.Server, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("mcp: token is required")
	}
	if port == "" {
		port = "8081"
	}
	if bind == "" {
		bind = "0.0.0.0"
	}

	svc := services.NewAppService(database, dataDir, func(ctx context.Context, level, msg string) {
		_ = database.AddLogWithAttrs(level, services.SanitizeLogMessage(msg), middleware.FromContext(ctx), services.RiskFromContext(ctx))
	})

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "shellyadmin",
		Version: version,
	}, nil)
	register(server, svc)

	streamable := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
		return server
	}, nil)

	handler := auth(token, requestIDMiddleware(streamable))

	httpSrv := &http.Server{
		Addr:              net.JoinHostPort(bind, port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return httpSrv, nil
}

// requestIDMiddleware ensures every MCP tool handler sees a stable
// X-Request-ID in its context. Honours a sane client-supplied header
// (8–64 chars of [A-Za-z0-9_-]); otherwise generates a fresh 16-char
// hex token. Echoes the value back on the response so operators can
// correlate audit rows with client-side traces.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := sanitizeRequestID(r.Header.Get(middleware.HeaderRequestID))
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(middleware.HeaderRequestID, id)
		ctx := middleware.WithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// sanitizeRequestID mirrors the alnum/dash/underscore rules used by
// internal/middleware/requestid.go's sanitizeInbound. We can't import
// that helper directly (it's unexported) so we duplicate the small
// validation rather than exporting it just for MCP.
func sanitizeRequestID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	const max = 64
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > max {
		trimmed = trimmed[:max]
	}
	for _, r := range trimmed {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '-' || r == '_':
		default:
			return ""
		}
	}
	return trimmed
}

func newRequestID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("rid-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
