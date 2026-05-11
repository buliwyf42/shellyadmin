// Package api wires HTTP handlers onto the gin router. The handler
// methods themselves are split across handler_*.go files by resource
// (auth, devices, scan+firmware, provision, settings, templates,
// credentials, logs+backup, meta). This file holds the Handler struct,
// constructor, audit-sink wiring, and a handful of in-package helpers
// (logReq, emitSlogWithRisk, RandomSecret, decodeJSON) shared across
// the per-resource files.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/db"
	"shellyadmin/internal/middleware"
	"shellyadmin/internal/services"
)

type Handler struct {
	db      *db.DB
	cfg     Config
	service *services.AppService
	// auditSink persists a single audit row. It's pluggable so tests can
	// capture output without standing up SQLite — the production wiring in
	// NewHandler sanitizes, mirrors to slog, then writes to the DB.
	auditSink func(level, msg, requestID string)
	// auditSinkAttrs is the structured variant; takes the catalog risk
	// level so action-execution rows can be filtered without parsing the
	// message body. Defaulted from auditSink in NewHandler so tests that
	// only stub auditSink keep working.
	auditSinkAttrs func(level, msg, requestID, riskLevel string)
	// logFn is the context-aware audit helper passed to services-layer
	// callbacks. When ctx carries a request ID (set by the RequestID
	// middleware), that ID flows into the audit row and slog line.
	logFn func(ctx context.Context, level, msg string)
}

func NewHandler(database *db.DB, cfg Config) *Handler {
	handler := &Handler{
		db:  database,
		cfg: cfg,
	}
	handler.auditSink = func(level, msg, reqID string) {
		handler.auditSinkAttrs(level, msg, reqID, "")
	}
	handler.auditSinkAttrs = func(level, msg, reqID, riskLevel string) {
		sanitized := services.SanitizeLogMessage(msg)
		emitSlogWithRisk(level, sanitized, reqID, riskLevel)
		_ = handler.db.AddLogWithAttrs(level, sanitized, reqID, riskLevel)
	}
	handler.logFn = func(ctx context.Context, level, msg string) {
		handler.auditSinkAttrs(level, msg, middleware.FromContext(ctx), services.RiskFromContext(ctx))
	}
	if cfg.Service != nil {
		// Reuse the externally-supplied AppService so background workers
		// (firmware-check scheduler) and the MCP controller share state
		// with HTTP handlers. The shared service was already constructed
		// with its own logFn; we leave it untouched.
		handler.service = cfg.Service
	} else {
		handler.service = services.NewAppService(database, cfg.DataDir, handler.logFn)
	}
	return handler
}

// logReq persists an audit entry tagged with the current request's
// correlation ID. Callers that already have a gin.Context should prefer this
// over h.logFn so the audit row links back to the originating request.
func (h *Handler) logReq(c *gin.Context, level, msg string) {
	h.auditSink(level, msg, middleware.FromGinContext(c))
}

// emitSlogWithRisk mirrors audit lines to the stdlib slog logger so
// operators tailing the container log see structured JSON rather than just
// the DB-persisted audit trail. The risk_level attribute is populated on
// action-execution rows so an operator grepping the container log can
// filter on it the same way SQLite queries do. Unknown levels fall back
// to info.
func emitSlogWithRisk(level, msg, reqID, riskLevel string) {
	attrs := []any{}
	if reqID != "" {
		attrs = append(attrs, slog.String("request_id", reqID))
	}
	if riskLevel != "" {
		attrs = append(attrs, slog.String("risk_level", riskLevel))
	}
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		slog.Debug(msg, attrs...)
	case "WARN", "WARNING":
		slog.Warn(msg, attrs...)
	case "ERROR":
		slog.Error(msg, attrs...)
	default:
		slog.Info(msg, attrs...)
	}
}

// RandomSecret returns a hex-encoded 32-byte random string, suitable for
// session nonces, CSRF tokens, and server-side session IDs.
func RandomSecret() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(buf)
}

// decodeJSON wraps http.MaxBytesReader + a strict JSON decoder. It
// rejects unknown fields and trailing content so a typo in the SPA
// payload surfaces as a 400 rather than silently being ignored.
func decodeJSON(c *gin.Context, dst any, maxBytes int64) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("unexpected trailing content")
		}
		return err
	}
	return nil
}
