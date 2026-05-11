package totp

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"shellyadmin/internal/core/secretbox"
	"shellyadmin/internal/db"
)

// fakeStore is a Store backed by a single in-memory row keyed on
// username. Lets the orchestration tests exercise the verify-and-
// advance state machine without standing up SQLite. Mirrors the row
// shape db.TOTPState produces so the bitmask + cipher columns
// round-trip identically.
type fakeStore struct {
	rows map[string]db.TOTPState
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]db.TOTPState{}} }

func (f *fakeStore) GetTOTP(username string) (db.TOTPState, error) {
	row, ok := f.rows[username]
	if !ok {
		return db.TOTPState{}, sql.ErrNoRows
	}
	return row, nil
}

func (f *fakeStore) SetTOTP(state db.TOTPState) error {
	f.rows[state.Username] = state
	return nil
}

func (f *fakeStore) DeleteTOTP(username string) error {
	delete(f.rows, username)
	return nil
}

// installTestKey sets the package-level secretbox key for the test
// run. The key store is process-global so concurrent tests would race;
// the totp service tests run serially via t.Parallel-free design.
func installTestKey(t *testing.T) {
	t.Helper()
	key, err := secretbox.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if err := secretbox.SetKey(key); err != nil {
		t.Fatalf("SetKey: %v", err)
	}
}

func TestBeginGeneratesPendingMaterial(t *testing.T) {
	installTestKey(t)
	svc := New(newFakeStore())
	mat, err := svc.Begin("admin")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if mat.Secret == "" {
		t.Errorf("Begin returned empty secret")
	}
	if len(mat.BackupCodes) != BackupCodes {
		t.Errorf("Begin returned %d backup codes, want %d", len(mat.BackupCodes), BackupCodes)
	}
	if len(mat.BackupHashesJSON) == 0 {
		t.Errorf("Begin returned empty hashes JSON")
	}
	// The otpauth URI must embed the account name so authenticator apps
	// label the entry correctly.
	if !contains(mat.OTPAuthURI, "admin") {
		t.Errorf("OTPAuthURI missing account name: %s", mat.OTPAuthURI)
	}
}

func TestCompleteHappyPath(t *testing.T) {
	installTestKey(t)
	store := newFakeStore()
	svc := New(store)
	fixed := time.Unix(1700000000, 0).UTC()
	svc.SetClock(func() time.Time { return fixed })

	mat, err := svc.Begin("admin")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	code, err := Generate(mat.Secret, fixed)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := svc.Complete("admin", mat.Secret, mat.BackupHashesJSON, code); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	row, ok := store.rows["admin"]
	if !ok {
		t.Fatalf("Complete did not write a row")
	}
	if row.SecretCipher == mat.Secret {
		t.Errorf("Complete persisted plaintext secret (should be sealed)")
	}
	if !secretbox.IsBlob(row.SecretCipher) {
		t.Errorf("secret cipher does not look like a secretbox blob: %q", row.SecretCipher)
	}
	if !secretbox.IsBlob(row.BackupCodesCipher) {
		t.Errorf("backup cipher does not look like a secretbox blob: %q", row.BackupCodesCipher)
	}
	if row.EnrolledAt == "" || row.LastVerifiedAt == "" {
		t.Errorf("Complete left timestamps empty: %+v", row)
	}
	if row.BackupCodesUsed != 0 {
		t.Errorf("Complete set BackupCodesUsed = %d, want 0", row.BackupCodesUsed)
	}
}

func TestCompleteRejectsWrongCode(t *testing.T) {
	installTestKey(t)
	svc := New(newFakeStore())
	mat, err := svc.Begin("admin")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	err = svc.Complete("admin", mat.Secret, mat.BackupHashesJSON, "000000")
	if !errors.Is(err, ErrInvalidCode) {
		t.Errorf("Complete with wrong code: got %v, want ErrInvalidCode", err)
	}
}

func TestStatusReportsBackupCodesLeft(t *testing.T) {
	installTestKey(t)
	store := newFakeStore()
	svc := New(store)
	fixed := time.Unix(1700000000, 0).UTC()
	svc.SetClock(func() time.Time { return fixed })

	mat, _ := svc.Begin("admin")
	code, _ := Generate(mat.Secret, fixed)
	if err := svc.Complete("admin", mat.Secret, mat.BackupHashesJSON, code); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	status, err := svc.Status("admin")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.Enrolled {
		t.Errorf("Status reported not enrolled after Complete")
	}
	if status.BackupCodesLeft != BackupCodes {
		t.Errorf("BackupCodesLeft = %d, want %d", status.BackupCodesLeft, BackupCodes)
	}

	// Burn one backup code via VerifyForLogin → BackupCodesLeft drops.
	usedBackup, err := svc.VerifyForLogin("admin", mat.BackupCodes[3])
	if err != nil {
		t.Fatalf("VerifyForLogin (backup): %v", err)
	}
	if !usedBackup {
		t.Errorf("VerifyForLogin returned usedBackup=false for backup code")
	}
	status, _ = svc.Status("admin")
	if status.BackupCodesLeft != BackupCodes-1 {
		t.Errorf("after burn, BackupCodesLeft = %d, want %d", status.BackupCodesLeft, BackupCodes-1)
	}
}

func TestStatusNotEnrolled(t *testing.T) {
	installTestKey(t)
	svc := New(newFakeStore())
	status, err := svc.Status("admin")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Enrolled {
		t.Errorf("Status reported enrolled for fresh user")
	}
}

func TestVerifyForLoginAcceptsTOTP(t *testing.T) {
	installTestKey(t)
	store := newFakeStore()
	svc := New(store)
	fixed := time.Unix(1700000000, 0).UTC()
	svc.SetClock(func() time.Time { return fixed })

	mat, _ := svc.Begin("admin")
	code, _ := Generate(mat.Secret, fixed)
	_ = svc.Complete("admin", mat.Secret, mat.BackupHashesJSON, code)

	// Same TOTP code is still within the skew window — must accept.
	usedBackup, err := svc.VerifyForLogin("admin", code)
	if err != nil {
		t.Fatalf("VerifyForLogin: %v", err)
	}
	if usedBackup {
		t.Errorf("VerifyForLogin reported backup for TOTP code")
	}
}

func TestVerifyForLoginRejectsReusedBackup(t *testing.T) {
	installTestKey(t)
	store := newFakeStore()
	svc := New(store)
	fixed := time.Unix(1700000000, 0).UTC()
	svc.SetClock(func() time.Time { return fixed })

	mat, _ := svc.Begin("admin")
	code, _ := Generate(mat.Secret, fixed)
	_ = svc.Complete("admin", mat.Secret, mat.BackupHashesJSON, code)

	// First use of backup code #2: ok.
	if _, err := svc.VerifyForLogin("admin", mat.BackupCodes[2]); err != nil {
		t.Fatalf("first VerifyForLogin (backup): %v", err)
	}
	// Second use of the SAME backup code: must be rejected as
	// ErrInvalidCode (NOT a distinct "already used" error — same shape
	// as a wrong code to avoid an enumeration oracle).
	_, err := svc.VerifyForLogin("admin", mat.BackupCodes[2])
	if !errors.Is(err, ErrInvalidCode) {
		t.Errorf("reused backup code: got %v, want ErrInvalidCode", err)
	}
}

func TestDisableRequiresValidCode(t *testing.T) {
	installTestKey(t)
	store := newFakeStore()
	svc := New(store)
	fixed := time.Unix(1700000000, 0).UTC()
	svc.SetClock(func() time.Time { return fixed })

	mat, _ := svc.Begin("admin")
	code, _ := Generate(mat.Secret, fixed)
	_ = svc.Complete("admin", mat.Secret, mat.BackupHashesJSON, code)

	// Wrong code → row stays.
	if err := svc.Disable("admin", "000000"); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("Disable with wrong code: got %v, want ErrInvalidCode", err)
	}
	if _, ok := store.rows["admin"]; !ok {
		t.Errorf("Disable wiped row despite wrong code")
	}

	// Right code → row gone.
	if err := svc.Disable("admin", code); err != nil {
		t.Fatalf("Disable with right code: %v", err)
	}
	if _, ok := store.rows["admin"]; ok {
		t.Errorf("Disable kept row despite valid code")
	}
}

func TestDisableNotEnrolled(t *testing.T) {
	installTestKey(t)
	svc := New(newFakeStore())
	err := svc.Disable("admin", "000000")
	if !errors.Is(err, ErrNotEnrolled) {
		t.Errorf("Disable on un-enrolled user: got %v, want ErrNotEnrolled", err)
	}
}

func TestCountUsedBits(t *testing.T) {
	cases := []struct {
		mask int
		want int
	}{
		{0, 0},
		{1, 1},
		{0b1010101010, 5},
		{(1 << BackupCodes) - 1, BackupCodes},
	}
	for _, c := range cases {
		if got := countUsedBits(c.mask); got != c.want {
			t.Errorf("countUsedBits(%b) = %d, want %d", c.mask, got, c.want)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
