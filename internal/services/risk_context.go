package services

import "context"

// ctxKeyRisk is a private context key so the value can only be set/read
// through the helpers below. Used by ExecuteDeviceAction to thread the
// catalog risk level through to the audit sink without changing the
// LogCtx signature every other caller depends on.
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
// WithRisk, or empty string if none. Used by the handler audit sink to
// populate audit_log.risk_level.
func RiskFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRisk{}).(string); ok {
		return v
	}
	return ""
}
