// Package loginlock owns the per-account rolling failure counter +
// lockout window the login handler gates on. Q20 — failure state is
// persisted in SQLite so a container restart doesn't reset the budget
// (in-memory state would be a bypass vector).
//
// MOVED FROM internal/services/app.go — v0.3.0 services-layer split (M7,
// docs/plans/phase-4b-refactor-block.md Block 4b.1). AppService keeps
// delegators on IsAccountLocked / RecordLoginFailure / RecordLoginSuccess
// so the existing auth handler + tests are unchanged.
package loginlock

import (
	"time"

	"shellyadmin/internal/db"
)

// MaxFailures is the consecutive-failure threshold that triggers a lockout.
// LockoutDur is how long the account stays locked. Once locked, a
// successful login is the only way to reset the counter; an expired
// lockout does NOT reset it because the next failure should re-lock
// immediately.
const (
	MaxFailures = 20
	LockoutDur  = 15 * time.Minute
)

// Store is the narrow persistence surface loginlock needs. *db.DB
// satisfies it structurally.
type Store interface {
	GetLoginState(username string) (db.LoginState, error)
	SetLoginState(state db.LoginState) error
}

// Service hosts the lockout state machine.
type Service struct {
	store Store
}

// New constructs a Service backed by the given store.
func New(store Store) *Service { return &Service{store: store} }

// IsLocked reports whether username is currently locked out from login
// attempts. The returned time is the wall-clock instant the lockout
// expires; meaningful only when locked == true.
func (s *Service) IsLocked(username string) (bool, time.Time) {
	state, err := s.store.GetLoginState(username)
	if err != nil || state.LockedUntil == "" {
		return false, time.Time{}
	}
	until, err := time.Parse(time.RFC3339, state.LockedUntil)
	if err != nil {
		return false, time.Time{}
	}
	if time.Now().UTC().Before(until) {
		return true, until
	}
	return false, time.Time{}
}

// RecordFailure increments the rolling failure counter for username. At
// MaxFailures consecutive failures the account is locked for LockoutDur.
func (s *Service) RecordFailure(username string) error {
	state, err := s.store.GetLoginState(username)
	if err != nil {
		return err
	}
	state.Username = username
	state.FailedCount++
	nowStr := time.Now().UTC().Format(time.RFC3339)
	state.LastFailedAt = nowStr
	if state.FailedCount >= MaxFailures {
		state.LockedUntil = time.Now().UTC().Add(LockoutDur).Format(time.RFC3339)
	}
	return s.store.SetLoginState(state)
}

// RecordSuccess clears the failure counter and lockout window for
// username so the next failure starts a fresh budget.
func (s *Service) RecordSuccess(username string) error {
	return s.store.SetLoginState(db.LoginState{Username: username})
}
