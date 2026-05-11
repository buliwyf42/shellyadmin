package services

// Delegators to internal/services/tokens. Kept on AppService during
// the v0.3.0 refactor cycle so the api package + middleware can reach
// the create / list / revoke / lookup surface without importing the
// tokens sub-package directly. See
// docs/plans/phase-4c-auth-strategics.md (Block 4c.2 / T3) and
// internal/services/tokens/tokens.go for the underlying implementation.

import (
	"shellyadmin/internal/db"
	"shellyadmin/internal/services/tokens"
)

// PATCreateResult re-exports the per-create payload (plaintext token
// + metadata) so the handler types its response body without pulling
// in the tokens sub-package.
type PATCreateResult = tokens.CreateResult

// ListedPAT re-exports the metadata-only row shape returned by
// ListPATs.
type ListedPAT = tokens.ListedPAT

// Sentinel errors re-exported so the handler + middleware can switch
// on them.
var (
	ErrPATInvalidToken = tokens.ErrInvalidToken
	ErrPATInvalidScope = tokens.ErrInvalidScope
	ErrPATEmptyScopes  = tokens.ErrEmptyScopes
	ErrPATNotFound     = tokens.ErrNotFound
)

// Scope catalog re-exports so callers don't need the tokens import
// just to reference a scope name.
const (
	ScopeAdmin         = tokens.ScopeAdmin
	ScopeDevicesRead   = tokens.ScopeDevicesRead
	ScopeDevicesWrite  = tokens.ScopeDevicesWrite
	ScopeFirmwareRead  = tokens.ScopeFirmwareRead
	ScopeFirmwareWrite = tokens.ScopeFirmwareWrite
	ScopeProvision     = tokens.ScopeProvision
	ScopeSettingsRead  = tokens.ScopeSettingsRead
	ScopeSettingsWrite = tokens.ScopeSettingsWrite
)

// AllPATScopes is the sorted catalog list, exposed to the SPA via the
// /api/tokens metadata response so the create-token form can render
// the checkbox list without duplicating the catalog.
func AllPATScopes() []string {
	out := make([]string, len(tokens.AllScopes))
	copy(out, tokens.AllScopes)
	return out
}

// CreatePAT mints a new Personal Access Token for username.
func (s *AppService) CreatePAT(username, name string, scopes []string, expiresInDays int) (PATCreateResult, error) {
	return s.tokens.Create(username, name, scopes, expiresInDays)
}

// ListPATs returns metadata for every PAT owned by username.
func (s *AppService) ListPATs(username string) ([]ListedPAT, error) {
	return s.tokens.List(username)
}

// RevokePAT marks the row revoked. username scopes ownership so a
// stolen cookie can't revoke a PAT belonging to a different operator.
func (s *AppService) RevokePAT(username, id string) error {
	return s.tokens.Revoke(username, id)
}

// LookupPAT is the middleware path. Verifies the bearer string,
// returns the username + parsed scopes on success. The middleware
// shape returns (username, scopes, error) so the auth-package
// interface stays free of *db imports. The full db.PAT row is
// available via LookupPATRow if a caller actually needs the metadata.
func (s *AppService) LookupPAT(rawToken string) (username string, scopes []string, err error) {
	row, scopes, err := s.tokens.Lookup(rawToken)
	if err != nil {
		return "", nil, err
	}
	return row.Username, scopes, nil
}

// LookupPATRow returns the full PAT row + scopes. Kept on the service
// for tests + future callers that want richer metadata (e.g. an
// audit-row decorator that wants the PAT's name in the log line).
func (s *AppService) LookupPATRow(rawToken string) (db.PAT, []string, error) {
	return s.tokens.Lookup(rawToken)
}

// HasPATScope is the policy helper. Returns true when `granted`
// satisfies `required` (either directly or via the `admin` wildcard).
func HasPATScope(granted []string, required string) bool {
	return tokens.HasScope(granted, required)
}
