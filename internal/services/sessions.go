package services

// Delegators to internal/services/sessions. Kept on AppService during the
// v0.3.0 refactor cycle so existing callers (api/handler_auth.go,
// api/router.go) compile unchanged. See docs/plans/phase-4b-refactor-block.md
// (Block 4b.1 / M7) and internal/services/sessions/sessions.go for the
// underlying implementation.

import (
	"shellyadmin/internal/services/sessions"
)

// SessionValidator returns the validator the auth middleware uses to check
// that a server-side session row is alive. Implements
// middleware.SessionValidator structurally.
func (s *AppService) SessionValidator() *sessions.Validator {
	return s.sessions.Validator()
}

// IssueSession creates a fresh session row for the Login handler.
func (s *AppService) IssueSession(sessionID, username string) (string, error) {
	return s.sessions.Issue(sessionID, username)
}

// RevokeSession marks a session row revoked for the Logout handler.
func (s *AppService) RevokeSession(sessionID string) error {
	return s.sessions.Revoke(sessionID)
}

// RevokeSessionsForUser revokes every active session belonging to username,
// used by the password-rotation path.
func (s *AppService) RevokeSessionsForUser(username string) error {
	return s.sessions.RevokeForUser(username)
}
