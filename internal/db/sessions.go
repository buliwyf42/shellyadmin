package db

// Server-side session rows (S5) + per-account login lockout state (Q20).
// MOVED FROM db.go — db-layer split by domain (post-v0.5.2 review item 6);
// bodies unchanged.

import (
	"database/sql"
	"time"
)

// LoginState is the failure-counter + lockout-window row backing Q20's
// per-account lockout. LockedUntil is "" when the account is unlocked.
type LoginState struct {
	Username     string
	FailedCount  int
	LastFailedAt string
	LockedUntil  string
}

// Session is the server-side anchor for a logged-in operator (S5). The
// cookie carries only the id; everything authoritative lives in this
// row. RevokedAt == "" means active; non-empty means the operator (or
// a forced revoke) signed out.
type Session struct {
	ID         string
	Username   string
	CreatedAt  string
	LastSeenAt string
	ExpiresAt  string
	RevokedAt  string
}

// CreateSession inserts a fresh session row. id must be a
// cryptographically random opaque token chosen by the caller (the
// Login handler uses RandomSecret() which gives 32 bytes of entropy).
// expiresAt is RFC3339 UTC.
func (db *DB) CreateSession(id, username, expiresAt string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.sql.Exec(
		`INSERT INTO sessions(id, username, created_at, last_seen_at, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, username, now, now, expiresAt,
	)
	return err
}

// GetSession returns the row for id. Returns sql.ErrNoRows when the
// session does not exist — the RequireAuth middleware treats that as
// "invalid session, redirect to login".
func (db *DB) GetSession(id string) (Session, error) {
	var s Session
	err := db.sql.QueryRow(
		`SELECT id, username, created_at, last_seen_at, expires_at, revoked_at
		 FROM sessions WHERE id = ?`,
		id,
	).Scan(&s.ID, &s.Username, &s.CreatedAt, &s.LastSeenAt, &s.ExpiresAt, &s.RevokedAt)
	return s, err
}

// TouchSession updates last_seen_at to now. Called by RequireAuth on
// every successful auth check so an idle session ages out via its
// expires_at but an active one keeps moving forward. The expires_at
// itself is NOT bumped here — sliding-window expiry is operator-
// policy that lives in app code.
func (db *DB) TouchSession(id string) error {
	_, err := db.sql.Exec(
		`UPDATE sessions SET last_seen_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), id,
	)
	return err
}

// RevokeSession is the logout path: flip revoked_at on a specific id.
// RequireAuth refuses subsequent reuses of the cookie even though the
// cookie's MaxAge has not elapsed. Idempotent.
func (db *DB) RevokeSession(id string) error {
	_, err := db.sql.Exec(
		`UPDATE sessions SET revoked_at = ? WHERE id = ? AND revoked_at = ''`,
		time.Now().UTC().Format(time.RFC3339), id,
	)
	return err
}

// RevokeAllForUser is the bulk path used when a password rotates: every
// active session for the user is invalidated so a stolen cookie cannot
// outlive the operator's intent.
func (db *DB) RevokeAllForUser(username string) error {
	_, err := db.sql.Exec(
		`UPDATE sessions SET revoked_at = ? WHERE username = ? AND revoked_at = ''`,
		time.Now().UTC().Format(time.RFC3339), username,
	)
	return err
}

// PruneExpiredSessions removes rows whose expires_at is in the past.
// Called by a background sweeper; missing it would leave the table
// growing unboundedly on a server that gets many short-lived logins
// (e.g. an operator opening dashboards from multiple devices).
func (db *DB) PruneExpiredSessions() (int64, error) {
	res, err := db.sql.Exec(
		`DELETE FROM sessions WHERE expires_at < ?`,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// GetLoginState returns the persisted counter for username, or a zero
// value (no failures) if no row exists yet. The zero value is the
// "never tried" state, treated as unlocked.
func (db *DB) GetLoginState(username string) (LoginState, error) {
	var ls LoginState
	ls.Username = username
	err := db.sql.QueryRow(
		`SELECT failed_count, last_failed_at, locked_until FROM login_state WHERE username = ?`,
		username,
	).Scan(&ls.FailedCount, &ls.LastFailedAt, &ls.LockedUntil)
	if err == sql.ErrNoRows {
		return ls, nil
	}
	if err != nil {
		return LoginState{}, err
	}
	return ls, nil
}

// SetLoginState upserts the row keyed by username.
func (db *DB) SetLoginState(state LoginState) error {
	_, err := db.sql.Exec(
		`INSERT INTO login_state(username, failed_count, last_failed_at, locked_until)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(username) DO UPDATE SET
			failed_count = excluded.failed_count,
			last_failed_at = excluded.last_failed_at,
			locked_until = excluded.locked_until`,
		state.Username, state.FailedCount, state.LastFailedAt, state.LockedUntil,
	)
	return err
}
