// Service orchestration over the TOTP primitives in this same package.
// The pure-crypto layer (totp.go) stays Store-free + stdlib-only; this
// file adds the persistence-aware enrollment / verify / disable flows
// that AppService delegates to.
//
// T1 from the consolidated review (docs/plans/phase-4c-auth-strategics.md,
// Block 4c.1). v0.3.0.
package totp

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"shellyadmin/internal/core/secretbox"
	"shellyadmin/internal/db"
)

// Issuer is the human-readable label embedded in the otpauth:// URI.
// Authenticator apps surface it as the account-row title alongside the
// account name. Kept as a package var so a future white-label deploy
// can override it without forking the handler.
var Issuer = "ShellyAdmin"

// Store is the narrow persistence surface needed by the orchestration
// layer. *db.DB satisfies it structurally — Service is constructed
// against the AppService-level Store so tests can substitute a fake.
type Store interface {
	GetTOTP(username string) (db.TOTPState, error)
	SetTOTP(state db.TOTPState) error
	DeleteTOTP(username string) error
}

// Service hosts the enrollment / verify / disable flows over the TOTP
// primitives.
type Service struct {
	store Store
	now   func() time.Time
}

// New constructs a Service backed by the given store. The wall-clock
// dependency is injected so tests can pin time without monkey-patching
// time.Now; production wiring leaves it nil and falls back to time.Now.
func New(store Store) *Service {
	return &Service{store: store}
}

// SetClock overrides the wall-clock source. Used by tests; production
// wiring never calls this.
func (s *Service) SetClock(fn func() time.Time) {
	s.now = fn
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now().UTC()
}

// EnrollmentMaterial is the pending-enrollment payload the SPA flashes
// at the operator after Begin and round-trips back into Complete. The
// secret + backup codes leave the process exactly once (in the Begin
// response body); the caller is responsible for stashing them in a
// session-scoped store until Complete fires.
type EnrollmentMaterial struct {
	// Secret is the base32 TOTP secret the operator's authenticator app
	// will key off. Plaintext — never store this column directly; the
	// service secretbox-seals it inside Complete.
	Secret string
	// OTPAuthURI is the otpauth:// URI for QR-code rendering.
	OTPAuthURI string
	// BackupCodes are the human-readable XXXX-XXXX one-time recovery
	// codes. The hashed-and-sealed counterpart lives in BackupHashesJSON.
	BackupCodes []string
	// BackupHashesJSON is the JSON-encoded sha256 list the caller passes
	// back into Complete. Treat as opaque — the wire shape is
	// `["hexhash", ...]`. NOT secretbox-sealed: Complete seals it before
	// committing to the DB.
	BackupHashesJSON []byte
}

// Status describes whether the operator has an active TOTP row.
// Surfaces the metadata the Settings UI card needs to render
// enroll-vs-disable + the "X of N backup codes remaining" line.
type Status struct {
	Enrolled        bool   `json:"enrolled"`
	EnrolledAt      string `json:"enrolled_at,omitempty"`
	LastVerifiedAt  string `json:"last_verified_at,omitempty"`
	BackupCodesLeft int    `json:"backup_codes_left,omitempty"`
}

// ErrNotEnrolled is returned by Status / Disable / VerifyForLogin when
// the operator has no totp_state row.
var ErrNotEnrolled = errors.New("totp: not enrolled")

// ErrInvalidCode is returned by Complete / Disable / VerifyForLogin
// when the supplied code matches neither the TOTP-of-the-moment nor
// any unused backup code. The login handler maps this to the same 401
// shape it returns for a wrong password (no enumeration oracle).
var ErrInvalidCode = errors.New("totp: invalid code")

// Begin mints a fresh TOTP secret + backup codes for username. Nothing
// is persisted; the caller stashes the returned material in a session-
// scoped cookie until Complete commits the enrollment. Returning the
// JSON-encoded hash list (rather than the hashes themselves) keeps the
// session-cookie wire format opaque.
func (s *Service) Begin(username string) (EnrollmentMaterial, error) {
	secret, err := GenerateSecret()
	if err != nil {
		return EnrollmentMaterial{}, fmt.Errorf("totp: generate secret: %w", err)
	}
	codes, err := GenerateBackupCodes(BackupCodes)
	if err != nil {
		return EnrollmentMaterial{}, fmt.Errorf("totp: generate backup codes: %w", err)
	}
	hashes := make([]string, len(codes))
	for i, code := range codes {
		hashes[i] = HashBackupCode(code)
	}
	hashesJSON, err := EncodeBackupHashes(hashes)
	if err != nil {
		return EnrollmentMaterial{}, fmt.Errorf("totp: encode backup hashes: %w", err)
	}
	return EnrollmentMaterial{
		Secret:           secret,
		OTPAuthURI:       OTPAuthURI(Issuer, username, secret),
		BackupCodes:      codes,
		BackupHashesJSON: hashesJSON,
	}, nil
}

// Complete commits the enrollment after the operator verified the
// supplied TOTP code against the in-flight secret. secretbox-seals
// both the secret and the backup-hash list at the boundary; the
// underlying DB row only ever sees ciphertext.
func (s *Service) Complete(username, secret string, backupHashesJSON []byte, code string) error {
	if !Verify(secret, code, s.clock()) {
		return ErrInvalidCode
	}
	sealedSecret, err := secretbox.SealString(secret)
	if err != nil {
		return fmt.Errorf("totp: seal secret: %w", err)
	}
	sealedHashes, err := secretbox.Seal(backupHashesJSON)
	if err != nil {
		return fmt.Errorf("totp: seal backup hashes: %w", err)
	}
	nowStr := s.clock().UTC().Format(time.RFC3339)
	return s.store.SetTOTP(db.TOTPState{
		Username:          username,
		SecretCipher:      sealedSecret,
		EnrolledAt:        nowStr,
		LastVerifiedAt:    nowStr,
		BackupCodesCipher: sealedHashes,
		BackupCodesUsed:   0,
	})
}

// Status returns the per-user enrollment summary used by the Settings UI.
// A user with no row is reported as enrolled=false (NOT an error); the
// caller treats that as "show the enroll button".
func (s *Service) Status(username string) (Status, error) {
	state, err := s.store.GetTOTP(username)
	if errors.Is(err, sql.ErrNoRows) {
		return Status{Enrolled: false}, nil
	}
	if err != nil {
		return Status{}, err
	}
	hashes, herr := s.decodeHashes(state.BackupCodesCipher)
	if herr != nil {
		return Status{}, herr
	}
	used := countUsedBits(state.BackupCodesUsed)
	left := len(hashes) - used
	if left < 0 {
		left = 0
	}
	return Status{
		Enrolled:        true,
		EnrolledAt:      state.EnrolledAt,
		LastVerifiedAt:  state.LastVerifiedAt,
		BackupCodesLeft: left,
	}, nil
}

// Disable revokes the operator's enrollment. Requires a fresh TOTP or
// unused backup code so an attacker who picked up the session cookie
// cannot quietly turn 2FA off. The backup-code path leaves the row in
// place only to mark the code used; the row is deleted regardless once
// either factor passes.
func (s *Service) Disable(username, code string) error {
	_, err := s.verifyAndAdvance(username, code)
	if err != nil {
		return err
	}
	return s.store.DeleteTOTP(username)
}

// VerifyForLogin checks code (TOTP or backup) for the login handler.
// On success the row's last_verified_at is bumped and any spent backup-
// code bit is set. usedBackup is true when the matched code came out of
// the recovery list (lets the caller surface the "1 less backup code
// available" warning to the operator).
func (s *Service) VerifyForLogin(username, code string) (usedBackup bool, err error) {
	return s.verifyAndAdvance(username, code)
}

// verifyAndAdvance is the shared body behind Disable + VerifyForLogin.
// Returns ErrNotEnrolled when the operator has no row, ErrInvalidCode
// when no factor matches, and a wrapped error on storage failure.
func (s *Service) verifyAndAdvance(username, code string) (bool, error) {
	state, err := s.store.GetTOTP(username)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotEnrolled
	}
	if err != nil {
		return false, err
	}
	secret, err := secretbox.OpenString(state.SecretCipher)
	if err != nil {
		return false, fmt.Errorf("totp: open secret: %w", err)
	}
	nowStr := s.clock().UTC().Format(time.RFC3339)
	if Verify(secret, code, s.clock()) {
		state.LastVerifiedAt = nowStr
		if err := s.store.SetTOTP(state); err != nil {
			return false, fmt.Errorf("totp: bump last_verified_at: %w", err)
		}
		return false, nil
	}
	// Backup code path. Only unused indexes count; verifying against an
	// already-used index returns the same ErrInvalidCode so an attacker
	// can't tell from the response whether a code was burned or never
	// existed.
	hashes, err := s.decodeHashes(state.BackupCodesCipher)
	if err != nil {
		return false, err
	}
	used := state.BackupCodesUsed
	idx, ok := VerifyBackupCode(hashes, code)
	if !ok || idx < 0 {
		return false, ErrInvalidCode
	}
	mask := 1 << idx
	if used&mask != 0 {
		return false, ErrInvalidCode
	}
	state.BackupCodesUsed = used | mask
	state.LastVerifiedAt = nowStr
	if err := s.store.SetTOTP(state); err != nil {
		return false, fmt.Errorf("totp: burn backup code: %w", err)
	}
	return true, nil
}

// decodeHashes opens the sealed backup-hash blob and parses the JSON.
// Returned slice is the same shape Begin produced; callers must NOT
// mutate it (used-state lives in the row's bitmask, not in the list).
func (s *Service) decodeHashes(cipher string) ([]string, error) {
	if cipher == "" {
		return nil, nil
	}
	plain, err := secretbox.Open(cipher)
	if err != nil {
		return nil, fmt.Errorf("totp: open backup hashes: %w", err)
	}
	var out []string
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, fmt.Errorf("totp: decode backup hashes: %w", err)
	}
	return out, nil
}

// countUsedBits is the popcount of an int over the first BackupCodes
// positions. Used by Status; an `int` with bits 0..9 set means all
// backup codes are spent.
func countUsedBits(mask int) int {
	n := 0
	for i := 0; i < BackupCodes; i++ {
		if mask&(1<<i) != 0 {
			n++
		}
	}
	return n
}
