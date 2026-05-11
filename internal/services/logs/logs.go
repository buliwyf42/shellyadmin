// Package logs is a thin sub-service over the persisted audit_log table.
// It exists mainly so AppService doesn't carry the trio of pass-through
// methods directly; the api/handler_logs.go path hits AppService
// delegators which forward here.
//
// MOVED FROM internal/services/app.go — v0.3.0 services-layer split (M7,
// docs/plans/phase-4b-refactor-block.md Block 4b.1).
package logs

import (
	"shellyadmin/internal/db"
)

// Store is the narrow persistence surface needed.
type Store interface {
	GetLogs(level, search string) ([]db.LogEntry, error)
	GetLogsFiltered(level, search, risk string) ([]db.LogEntry, error)
	ClearLogs() (int64, error)
}

// Service hosts the read + clear API.
type Service struct {
	store Store
}

// New constructs a Service backed by the given store.
func New(store Store) *Service { return &Service{store: store} }

// Get returns the audit-log rows filtered by level + search.
func (s *Service) Get(level, search string) ([]db.LogEntry, error) {
	return s.store.GetLogs(level, search)
}

// GetFiltered returns audit-log rows with the additional risk-level filter
// (the MCP / per-action audit rows carry a risk_level column that callers
// can use to find only "high" risk events).
func (s *Service) GetFiltered(level, search, risk string) ([]db.LogEntry, error) {
	return s.store.GetLogsFiltered(level, search, risk)
}

// Clear truncates the audit_log table and returns the deleted row count.
func (s *Service) Clear() (int64, error) {
	return s.store.ClearLogs()
}
