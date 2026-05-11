package middleware

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// StructuredLogger is a drop-in for gin.Logger() that emits each request as
// a single slog.Info line (method, status, latency, client ip, sanitized
// path). Differences vs. gin.Default():
//
//   - Output goes through slog.Default(), so it lands in the same JSON file
//     and stderr sink as every other structured log line.
//   - The path is sanitized: query string is stripped (it can carry
//     sensitive values like `?search=...` or accidental `?token=...` if
//     a misconfigured client tries that pattern). Only the request URI
//     path remains.
//   - The request ID from the RequestID middleware is included so request
//     lines pair with audit rows by the same correlation key.
//
// The MCP HTTP listener runs on a separate *http.Server (not this Gin
// router) so its URL-form path-token never reaches this logger, but we
// still strip queries here as defense-in-depth.
func StructuredLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)

		path := c.Request.URL.Path
		// Defensive: a stray `?` shouldn't appear in URL.Path (it would
		// be in URL.RawQuery), but trim if some upstream embedded it.
		if i := strings.IndexByte(path, '?'); i >= 0 {
			path = path[:i]
		}

		attrs := []any{
			slog.String("method", c.Request.Method),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("latency", latency),
			slog.String("path", path),
			slog.String("client_ip", c.ClientIP()),
		}
		if reqID := FromGinContext(c); reqID != "" {
			attrs = append(attrs, slog.String("request_id", reqID))
		}
		if errs := c.Errors.ByType(gin.ErrorTypePrivate); len(errs) > 0 {
			attrs = append(attrs, slog.String("error", errs.String()))
		}
		slog.Info("http_request", attrs...)
	}
}
