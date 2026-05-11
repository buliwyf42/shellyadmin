package services

import (
	"database/sql"
	"errors"
	"time"
)

// sessionValidator adapts the AppService's Store onto the
// middleware.SessionValidator interface. AppService owns it so the
// middleware package stays free of *db.DB imports.
type sessionValidator struct {
	store Store
}

// SessionValidator returns the validator the auth middleware uses to
// check that a server-side session row is alive. Implements
// middleware.SessionValidator.
func (s *AppService) SessionValidator() *sessionValidator {
	return &sessionValidator{store: s.db}
}

// ValidateSession returns ok=true only when the row exists, is not
// revoked, and is not expired. A missing row (sql.ErrNoRows) is a
// quiet "ok=false, err=nil" — RequireAuth treats that as logged-out.
// Any other error bubbles up so the middleware can refuse the request
// on storage failure (fail-closed).
func (v *sessionValidator) ValidateSession(id string) (bool, error) {
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
// returned but the middleware swallows them on the hot path. Used so
// future operator audits can see "this session has been quiet for X
// hours" without re-issuing the cookie itself.
func (v *sessionValidator) TouchSession(id string) error {
	if v == nil || v.store == nil || id == "" {
		return nil
	}
	return v.store.TouchSession(id)
}

// IssueSession is the Login handler's path: create a fresh session
// row whose id matches the session cookie's "session_id" value.
// Returns the expires_at the caller should record (so a future
// /api/sessions endpoint can show the operator their active devices).
func (s *AppService) IssueSession(sessionID, username string) (string, error) {
	expires := time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	if err := s.db.CreateSession(sessionID, username, expires); err != nil {
		return "", err
	}
	return expires, nil
}

// RevokeSession is the Logout handler's path.
func (s *AppService) RevokeSession(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	return s.db.RevokeSession(sessionID)
}

// RevokeSessionsForUser is the password-rotation path. Currently
// unused (no change-password endpoint in v0.2.x) but kept on the
// surface so a future password-rotation handler can call it.
func (s *AppService) RevokeSessionsForUser(username string) error {
	return s.db.RevokeAllForUser(username)
}
