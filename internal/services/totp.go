package services

// Delegators to internal/services/totp. Kept on AppService during the
// v0.3.0 refactor cycle so the api package can reach the enrollment /
// status / disable / login-verify surface without importing the
// totp sub-package directly. See docs/plans/phase-4c-auth-strategics.md
// (Block 4c.1 / T1) and internal/services/totp/service.go for the
// underlying implementation.

import (
	"shellyadmin/internal/services/totp"
)

// TOTPEnrollmentMaterial re-exports the per-enrollment payload so the
// handler can type the session-stash + response without pulling in the
// totp sub-package.
type TOTPEnrollmentMaterial = totp.EnrollmentMaterial

// TOTPStatus re-exports the Status struct returned by GET /api/totp/status.
type TOTPStatus = totp.Status

// Sentinel errors re-exported so the handler can switch on them.
var (
	ErrTOTPNotEnrolled = totp.ErrNotEnrolled
	ErrTOTPInvalidCode = totp.ErrInvalidCode
)

// BeginTOTPEnrollment mints a fresh secret + backup codes for username.
// Nothing is persisted; the handler stashes the returned material in a
// session-scoped cookie until CompleteTOTPEnrollment fires.
func (s *AppService) BeginTOTPEnrollment(username string) (TOTPEnrollmentMaterial, error) {
	return s.totp.Begin(username)
}

// CompleteTOTPEnrollment commits the enrollment after the operator's
// authenticator app has produced a valid code against the pending
// secret. The secret + backup-hash list are secretbox-sealed at this
// boundary; the underlying DB row only ever sees ciphertext.
func (s *AppService) CompleteTOTPEnrollment(username, secret string, backupHashesJSON []byte, code string) error {
	return s.totp.Complete(username, secret, backupHashesJSON, code)
}

// TOTPStatusFor returns the per-user enrollment summary used by the
// Settings UI card.
func (s *AppService) TOTPStatusFor(username string) (TOTPStatus, error) {
	return s.totp.Status(username)
}

// DisableTOTP revokes the operator's TOTP row. Requires a fresh TOTP
// or unused backup code so a stolen session cookie cannot quietly
// disable 2FA.
func (s *AppService) DisableTOTP(username, code string) error {
	return s.totp.Disable(username, code)
}

// VerifyTOTPForLogin is the hook the Login handler calls when the
// operator submitted a totp_code alongside their password. Returns
// usedBackup=true when the matched factor was a recovery code so the
// caller can surface the "1 less backup code available" warning.
func (s *AppService) VerifyTOTPForLogin(username, code string) (bool, error) {
	return s.totp.VerifyForLogin(username, code)
}
