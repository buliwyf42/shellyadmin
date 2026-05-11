package services

// Backward-compat aliases — the implementation moved to
// internal/services/audit in v0.3.0 (M7,
// docs/plans/phase-4b-refactor-block.md Block 4b.1). Existing tests
// (audit_webhook_test.go) reference these names directly.

import (
	"shellyadmin/internal/services/audit"
)

// AuditWebhookEvent re-exports the wire shape audit posts to operator
// webhooks.
type AuditWebhookEvent = audit.AuditWebhookEvent

// shouldForward delegates to internal/services/audit.ShouldForward.
func shouldForward(level, minLevel string) bool {
	return audit.ShouldForward(level, minLevel)
}

// validateWebhookURL delegates to internal/services/audit.ValidateWebhookURL,
// kept on the package so audit_webhook_test.go's existing test names
// continue to resolve.
func validateWebhookURL(raw string) error { return audit.ValidateWebhookURL(raw) }
