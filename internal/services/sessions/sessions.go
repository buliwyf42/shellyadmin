// Package sessions owns server-side session row lifecycle: issue, revoke,
// and validate on every authenticated request.
//
// MOVED FROM internal/services/sessions.go — v0.3.0 services-layer split (M7,
// docs/plans/phase-4b-refactor-block.md Block 4b.1). Delegators on
// *services.AppService preserve the public surface (IssueSession /
// RevokeSession / RevokeSessionsForUser / SessionValidator) so existing
// api/handler_auth.go and api/router.go call sites are unchanged.
package sessions

import (
	"database/sql"
	"errors"
	"time"

	"shellyadmin/internal/db"
)

// Store is the narrow persistence surface needed by the sessions sub-service.
// *db.DB satisfies it structurally; tests can substitute a fake without
// implementing the full services.Store interface.
type Store interface {
	CreateSession(id, username, expiresAt string) error
	GetSession(id string) (db.Session, error)
	TouchSession(id string) error
	RevokeSession(id string) error
	RevokeAllForUser(username string) error
}

// Service owns session lifecycle. Construct via New and let the embedding
// service (AppService) delegate the public methods to it.
type Service struct {
	store Store
}

// New constructs a Service backed by the given store.
func New(store Store) *Service { return &Service{store: store} }

// Validator adapts the Store onto middleware.SessionValidator. Returned by
// Service.Validator so the auth middleware stays free of *db.DB imports.
type Validator struct {
	store Store
}

// Validator returns the validator the auth middleware uses to check that a
// server-side session row is alive.
func (s *Service) Validator() *Validator {
	if s == nil {
		return nil
	}
	return &Validator{store: s.store}
}

// ValidateSession returns ok=true only when the row exists, is not revoked,
// and is not expired. A missing row (sql.ErrNoRows) is a quiet
// "ok=false, err=nil" — RequireAuth treats that as logged-out. Any other
// error bubbles up so the middleware can refuse the request on storage
// failure (fail-closed).
func (v *Validator) ValidateSession(id string) (bool, error) {
	if v == nil || v.store == nil || id == "" {
		return false, nil
	}
	row, err := v.store.GetSession(id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row.RevokedAt != "" {
		return false, nil
	}
	if row.ExpiresAt != "" {
		exp, perr := time.Parse(time.RFC3339, row.ExpiresAt)
		if perr == nil && time.Now().UTC().After(exp) {
			return false, nil
		}
	}
	return true, nil
}

// TouchSession bumps last_seen_at on the row. Best-effort: errors are
// returned but the middleware swallows them on the hot path. Used so a
// future operator audit can see "this session has been quiet for X hours"
// without re-issuing the cookie itself.
func (v *Validator) TouchSession(id string) error {
	if v == nil || v.store == nil || id == "" {
		return nil
	}
	return v.store.TouchSession(id)
}

// Issue is the Login handler's path: create a fresh session row whose id
// matches the session cookie's "session_id" value. Returns the expires_at
// the caller should record (so a future /api/sessions endpoint can show the
// operator their active devices).
func (s *Service) Issue(sessionID, username string) (string, error) {
	expires := time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	if err := s.store.CreateSession(sessionID, username, expires); err != nil {
		return "", err
	}
	return expires, nil
}

// Revoke is the Logout handler's path.
func (s *Service) Revoke(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	return s.store.RevokeSession(sessionID)
}

// RevokeForUser is the password-rotation path. Currently unused (no
// change-password endpoint in v0.2.x) but kept on the surface so a future
// rotation handler can call it.
func (s *Service) RevokeForUser(username string) error {
	return s.store.RevokeAllForUser(username)
}
