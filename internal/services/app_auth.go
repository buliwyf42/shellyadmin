package services

import (
	"errors"
	"strings"
)

// ErrAuthAlreadyConfigured is returned by SetupAdminCredential when an
// operator login already exists. The first-run setup endpoint maps it to
// HTTP 409 so a second setup attempt cannot silently overwrite the account.
var ErrAuthAlreadyConfigured = errors.New("admin credential already configured")

// AdminCredential resolves the operator login from the database. configured
// is false when no row exists yet — the "boot into setup mode" state. The
// returned passHash is an argon2id PHC string suitable for VerifyPassword.
//
// Reads the single-row table directly on each call rather than caching: the
// query is cheap next to the ~80 ms argon2 verify it precedes, and a shared
// AppService (HTTP + MCP + stdio) sidesteps any cache-invalidation race.
func (s *AppService) AdminCredential() (username, passHash string, configured bool) {
	u, h, ok, err := s.db.GetAdminCredential()
	if err != nil {
		s.Log("ERROR", "read admin credential: "+err.Error())
		return "", "", false
	}
	return u, h, ok
}

// IsAuthConfigured reports whether an operator login has been set. False
// means the server is in setup mode.
func (s *AppService) IsAuthConfigured() bool {
	_, _, ok := s.AdminCredential()
	return ok
}

// SetupAdminCredential is the first-run path: hashes plain and stores it as
// the operator login, but only when none is configured yet. Returns
// ErrAuthAlreadyConfigured if a row already exists so the setup endpoint is
// a one-shot. username defaults to "admin" when blank.
func (s *AppService) SetupAdminCredential(username, plain string) error {
	username = normalizeUsername(username)
	hash, err := HashPassword(plain)
	if err != nil {
		return err
	}
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if _, _, ok, err := s.db.GetAdminCredential(); err != nil {
		return err
	} else if ok {
		return ErrAuthAlreadyConfigured
	}
	if err := s.db.SaveAdminCredential(username, hash); err != nil {
		return err
	}
	s.Log("INFO", "admin credential configured via first-run setup (user="+username+")")
	return nil
}

// ChangeAdminCredential updates the stored operator login. Used by the
// authenticated Settings flow after the caller has verified the current
// password. username defaults to "admin" when blank.
func (s *AppService) ChangeAdminCredential(username, plain string) error {
	username = normalizeUsername(username)
	hash, err := HashPassword(plain)
	if err != nil {
		return err
	}
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if err := s.db.SaveAdminCredential(username, hash); err != nil {
		return err
	}
	s.Log("WARN", "admin credential changed (user="+username+")")
	return nil
}

// ClearAdminCredential removes the stored operator login, returning the
// server to setup mode on the next boot. Backs the `shellyctl reset-auth`
// recovery subcommand.
func (s *AppService) ClearAdminCredential() error {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	return s.db.ClearAdminCredential()
}

// ImportEnvCredentialOnce migrates a legacy SHELLYADMIN_PASS_HASH into the
// database exactly once: only when no credential is configured yet AND a
// non-empty env hash is supplied. Returns true when an import happened. This
// is the seamless-upgrade path for deployments that still set the env hash —
// the existing password keeps working and the env var becomes irrelevant
// afterwards.
func (s *AppService) ImportEnvCredentialOnce(envUser, envPassHash string) (bool, error) {
	envPassHash = strings.TrimSpace(envPassHash)
	if envPassHash == "" {
		return false, nil
	}
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if _, _, ok, err := s.db.GetAdminCredential(); err != nil {
		return false, err
	} else if ok {
		return false, nil
	}
	username := normalizeUsername(envUser)
	if err := s.db.SaveAdminCredential(username, envPassHash); err != nil {
		return false, err
	}
	s.Log("INFO", "imported SHELLYADMIN_PASS_HASH into the database (user="+username+"); the env var is no longer the source of truth")
	return true, nil
}

func normalizeUsername(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return "admin"
	}
	return username
}
