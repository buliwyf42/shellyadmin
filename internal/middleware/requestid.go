package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/gin-gonic/gin"
)

// HeaderRequestID is the HTTP header used to carry the request correlation ID
// both inbound (echoed if the client provides it) and outbound.
const HeaderRequestID = "X-Request-ID"

// ginContextKey is the gin.Context key used to stash the current request ID.
const ginContextKey = "shellyadmin.request_id"

// requestIDCtxKey is the typed key used for the stdlib context.Context so
// non-gin callers (services, long-running jobs) can retrieve the ID without
// leaking the gin type.
type requestIDCtxKey struct{}

// maxInboundLen bounds how much of a client-supplied X-Request-ID we accept
// and store. Keeps the audit log tidy and prevents abuse.
const maxInboundLen = 64

// RequestID returns middleware that ensures every authenticated request has a
// stable correlation ID. Honours a client-supplied X-Request-ID when present
// (alnum/dash/underscore only, truncated to maxInboundLen); otherwise
// generates a fresh 16-hex-char token. The value is exposed on both the gin
// context and the stdlib request context, and echoed back in the response
// header for tail-the-logs debugging.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := sanitizeInbound(c.GetHeader(HeaderRequestID))
		if id == "" {
			id = newRequestID()
		}
		c.Set(ginContextKey, id)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDCtxKey{}, id))
		c.Writer.Header().Set(HeaderRequestID, id)
		c.Next()
	}
}

// FromGinContext returns the request ID associated with the current request,
// or "" if the middleware did not run for this route.
func FromGinContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if value, ok := c.Get(ginContextKey); ok {
		if id, ok := value.(string); ok {
			return id
		}
	}
	return ""
}

// FromContext mirrors FromGinContext for callers that only have a stdlib
// context.Context (background jobs, services layer).
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if value, ok := ctx.Value(requestIDCtxKey{}).(string); ok {
		return value
	}
	return ""
}

// WithRequestID returns ctx augmented with the given request ID. Useful when a
// background goroutine needs to carry the originating request's correlation
// ID through work that outlives the HTTP round-trip.
func WithRequestID(ctx context.Context, id string) context.Context {
	if ctx == nil || id == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDCtxKey{}, id)
}

func newRequestID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "rid-fallback"
	}
	return hex.EncodeToString(buf[:])
}

func sanitizeInbound(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > maxInboundLen {
		trimmed = trimmed[:maxInboundLen]
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
