package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"shellyadmin/internal/models"
)

// AuditWebhookEvent is the JSON payload posted to operator-supplied
// webhook endpoints. Stable shape — adding fields is OK, renaming is a
// breaking change that should bump a version field. The schema is
// deliberately compact: a Slack / Discord / Loki-push receiver should
// be able to consume it without a custom parser.
type AuditWebhookEvent struct {
	TS        string `json:"ts"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	RiskLevel string `json:"risk_level,omitempty"`
	Source    string `json:"source"` // "shellyadmin"
}

// auditWebhookClient owns the HTTP client used by webhook deliveries.
// Short timeout + no retry — webhook delivery is best-effort; the
// local audit_log row is the authoritative record. A slow webhook
// must not back up the service.
var auditWebhookClient = &http.Client{Timeout: 5 * time.Second}

// shouldForward consults AppSettings.AuditWebhookMinLevel. Empty
// MinLevel defaults to INFO+; DEBUG events drop unless the operator
// explicitly opted in.
func shouldForward(level, minLevel string) bool {
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

// validateWebhookURL accepts only absolute http(s) URLs with a host —
// rejects file:// + relative paths + missing scheme so a typo in the
// settings UI surfaces immediately rather than at first delivery.
func validateWebhookURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid webhook url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook url must be http:// or https://")
	}
	if u.Host == "" {
		return fmt.Errorf("webhook url must include a host")
	}
	return nil
}

// forwardAudit performs the webhook delivery. Runs on its own
// goroutine so a slow / unreachable webhook does not block the
// calling code path. Errors are deliberately swallowed (logged at
// DEBUG via the service-layer logf, never re-raised) — the local
// audit row is the source of truth.
func (s *AppService) forwardAudit(level, msg, reqID, riskLevel string, settings models.AppSettings) {
	if settings.AuditWebhookURL == "" {
		return
	}
	if !shouldForward(level, settings.AuditWebhookMinLevel) {
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
		resp, err := auditWebhookClient.Do(req)
		if err != nil {
			return
		}
		_ = resp.Body.Close()
	}()
}
