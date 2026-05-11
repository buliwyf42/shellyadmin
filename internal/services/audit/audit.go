// Package audit owns the service-level audit-log sink: secret-sanitized
// formatting, optional webhook forwarding (T11), and the context-bound
// risk-level marker every action passes through.
//
// MOVED FROM internal/services/{app.go, audit_webhook.go, risk_context.go}
// — v0.3.0 services-layer split (M7, docs/plans/phase-4b-refactor-block.md
// Block 4b.1). The services package keeps aliases on SanitizeLogMessage,
// WithRisk, RiskFromContext, AuditWebhookEvent so existing call sites in
// internal/{mcp,api} and cmd/shellyctl compile unchanged.
package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"shellyadmin/internal/middleware"
	"shellyadmin/internal/models"
	"shellyadmin/internal/services/validation"
)

// AuditWebhookEvent is the JSON payload posted to operator-supplied
// webhook endpoints. Stable shape — adding fields is OK, renaming is a
// breaking change that should bump a version field. The schema is
// deliberately compact: a Slack / Discord / Loki-push receiver should be
// able to consume it without a custom parser.
type AuditWebhookEvent struct {
	TS        string `json:"ts"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	RiskLevel string `json:"risk_level,omitempty"`
	Source    string `json:"source"` // "shellyadmin"
}

// secretPattern matches the three forms credentials appear in our log
// pipeline: `password=plain`, `password: plain`, and JSON-quoted
// `"password":"plain"`. The optional `["']?` after the key handles the
// JSON case where the quote follows the field name. S21 added regression
// tests in sanitize_log_test.go — extending the keyword set requires a
// matching test case there.
var secretPattern = regexp.MustCompile(`(?i)(password|pass|secret|ha1)["']?\s*[:=]\s*("[^"]*"|[^,\s\}\)&]+)`)

// SanitizeLogMessage redacts credential-shaped substrings from msg before
// it's written to the audit sink. Returns the redacted form.
func SanitizeLogMessage(msg string) string {
	return secretPattern.ReplaceAllString(msg, `$1=[redacted]`)
}

// ctxKeyRisk is a private context key so the value can only be
// set/read through the helpers below.
type ctxKeyRisk struct{}

// WithRisk wraps ctx so any audit lines emitted while running an action
// carry the catalog risk level (e.g. "low" / "medium" / "high"). The
// audit sink reads this back via RiskFromContext.
func WithRisk(ctx context.Context, risk string) context.Context {
	if risk == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyRisk{}, risk)
}

// RiskFromContext returns the risk level previously attached to ctx by
// WithRisk, or empty string if none.
func RiskFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRisk{}).(string); ok {
		return v
	}
	return ""
}

// Store is the narrow persistence surface audit needs (GetSettings is the
// only call, used to read the webhook URL + min-level + the operator's
// "audit_webhook_enabled" flag).
type Store interface {
	GetSettings() (models.AppSettings, error)
}

// MetricSink lets the audit sink count log emissions by level. Nil-safe
// at call time so AppService can pass nil for tests that don't wire
// metrics up.
type MetricSink interface {
	IncLabelled(name string, labels map[string]string)
}

// Service hosts the audit sink methods.
type Service struct {
	store   Store
	logf    func(ctx context.Context, level, msg string)
	metrics MetricSink
	client  *http.Client
}

// webhookClient owns the HTTP client used by webhook deliveries. Short
// timeout + no retry — webhook delivery is best-effort; the local
// audit_log row is the authoritative record. A slow webhook must not
// back up the service.
var webhookClient = &http.Client{Timeout: 5 * time.Second}

// New constructs a Service. logf is the per-row audit-row writer the host
// installs (its callback ultimately calls db.AddLog). metrics may be nil.
func New(store Store, logf func(ctx context.Context, level, msg string), metrics MetricSink) *Service {
	if logf == nil {
		logf = func(context.Context, string, string) {}
	}
	return &Service{store: store, logf: logf, metrics: metrics, client: webhookClient}
}

// Log emits an audit entry without a request-scoped context. Prefer
// LogCtx when a context is in scope so the audit row can be correlated
// back to the originating HTTP request. This form remains for callbacks
// passed to external packages (scanner, firmware) that use the narrower
// signature.
func (s *Service) Log(level, msg string) {
	s.LogCtx(context.Background(), level, msg)
}

// LogCtx emits an audit entry carrying the given context. The handler
// callback pulls the request ID + risk level out of ctx so the audit_log
// row links back to the originating HTTP request.
func (s *Service) LogCtx(ctx context.Context, level, msg string) {
	if s.metrics != nil {
		s.metrics.IncLabelled("shellyadmin_audit_rows_written_total", map[string]string{"level": strings.ToUpper(strings.TrimSpace(level))})
	}
	sanitized := SanitizeLogMessage(msg)
	s.logf(ctx, level, sanitized)
	s.maybeForwardAudit(ctx, level, sanitized)
}

// maybeForwardAudit shells out to the audit webhook delivery code if the
// operator configured one. Best-effort: errors are swallowed (the local
// audit_log row is the source of truth; the webhook is a replica).
// Reads settings on every call because the webhook URL can change at
// runtime via /api/settings — the operator should not have to restart
// the service to disable a forwarder.
func (s *Service) maybeForwardAudit(ctx context.Context, level, msg string) {
	if s.store == nil {
		return
	}
	settings, err := s.store.GetSettings()
	if err != nil || settings.AuditWebhookURL == "" {
		return
	}
	reqID := ""
	risk := ""
	if ctx != nil {
		reqID = middleware.FromContext(ctx)
		risk = RiskFromContext(ctx)
	}
	s.forwardAudit(level, msg, reqID, risk, settings)
}

// ShouldForward consults AppSettings.AuditWebhookMinLevel. Empty MinLevel
// defaults to INFO+; DEBUG events drop unless the operator explicitly
// opted in.
func ShouldForward(level, minLevel string) bool {
	rank := map[string]int{"DEBUG": 0, "INFO": 1, "WARN": 2, "ERROR": 3}
	lvl, ok := rank[strings.ToUpper(strings.TrimSpace(level))]
	if !ok {
		lvl = 1 // unknown → treat as INFO
	}
	floor, ok := rank[strings.ToUpper(strings.TrimSpace(minLevel))]
	if !ok {
		floor = 1 // empty / unknown → INFO+
	}
	return lvl >= floor
}

// forwardAudit performs the webhook delivery. Runs on its own goroutine
// so a slow / unreachable webhook does not block the calling code path.
// Errors are deliberately swallowed (the local audit row is the source
// of truth).
func (s *Service) forwardAudit(level, msg, reqID, riskLevel string, settings models.AppSettings) {
	if settings.AuditWebhookURL == "" {
		return
	}
	if !ShouldForward(level, settings.AuditWebhookMinLevel) {
		return
	}
	event := AuditWebhookEvent{
		TS:        time.Now().UTC().Format(time.RFC3339),
		Level:     strings.ToUpper(strings.TrimSpace(level)),
		Message:   msg,
		RequestID: reqID,
		RiskLevel: riskLevel,
		Source:    "shellyadmin",
	}
	body, err := json.Marshal(event)
	if err != nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.AuditWebhookURL, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "shellyadmin-audit-webhook/1.0")
		resp, err := s.client.Do(req)
		if err != nil {
			return
		}
		_ = resp.Body.Close()
	}()
}

// ValidateWebhookURL is a re-export of validation.WebhookURL. Kept here
// so audit-side test files (audit_webhook_test.go in services) can find
// the validator on the audit package's surface.
func ValidateWebhookURL(raw string) error { return validation.WebhookURL(raw) }
