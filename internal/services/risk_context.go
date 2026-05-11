package services

// Risk-context helpers moved to internal/services/audit in v0.3.0 (M7).
// The two helpers below are thin re-exports so cmd/shellyctl/main.go,
// internal/mcp, and internal/api/handler.go keep using services.WithRisk
// / services.RiskFromContext unchanged.

import (
	"context"

	"shellyadmin/internal/services/audit"
)

// WithRisk forwards to internal/services/audit.WithRisk.
func WithRisk(ctx context.Context, risk string) context.Context {
	return audit.WithRisk(ctx, risk)
}

// RiskFromContext forwards to internal/services/audit.RiskFromContext.
func RiskFromContext(ctx context.Context) string {
	return audit.RiskFromContext(ctx)
}
