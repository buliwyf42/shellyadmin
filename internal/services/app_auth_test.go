package services

import (
	"context"
	"testing"

	"shellyadmin/internal/db"
)

func newAuthTestService(t *testing.T) (*AppService, *fakeStore) {
	t.Helper()
	fake := newFakeStore()
	svc := NewAppService(fake, t.TempDir(), func(context.Context, string, string) {})
	return svc, fake
}

// TestImportEnvCredentialOnce_OnlyOnEmptyDB locks in the seamless-upgrade
// contract: the env hash is imported only when no credential exists, and only
// once.
func TestImportEnvCredentialOnce_OnlyOnEmptyDB(t *testing.T) {
	svc, fake := newAuthTestService(t)

	imported, err := svc.ImportEnvCredentialOnce("admin", "$argon2id$env-hash")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !imported {
		t.Fatalf("expected import on empty DB")
	}
	if fake.adminHash != "$argon2id$env-hash" || fake.adminUser != "admin" {
		t.Fatalf("stored credential = %q/%q, want admin/$argon2id$env-hash", fake.adminUser, fake.adminHash)
	}

	// Second import is a no-op even with a different hash.
	imported, err = svc.ImportEnvCredentialOnce("admin", "$argon2id$other-hash")
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if imported {
		t.Fatalf("expected no import when a credential already exists")
	}
	if fake.adminHash != "$argon2id$env-hash" {
		t.Fatalf("credential overwritten by second import: %q", fake.adminHash)
	}
}

// TestImportEnvCredentialOnce_EmptyHashIsNoop confirms setup-mode boot (no env
// hash) does not write anything.
func TestImportEnvCredentialOnce_EmptyHashIsNoop(t *testing.T) {
	svc, fake := newAuthTestService(t)
	imported, err := svc.ImportEnvCredentialOnce("admin", "")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported || fake.adminOK {
		t.Fatalf("expected no import for an empty env hash")
	}
	if svc.IsAuthConfigured() {
		t.Fatalf("expected unconfigured after a no-op import")
	}
}

// TestSetupAdminCredential_OneShot proves setup refuses to overwrite an
// existing account.
func TestSetupAdminCredential_OneShot(t *testing.T) {
	svc, _ := newAuthTestService(t)
	if err := svc.SetupAdminCredential("operator", "hunter2hunter2"); err != nil {
		t.Fatalf("first setup: %v", err)
	}
	if err := svc.SetupAdminCredential("attacker", "differentpass"); err != ErrAuthAlreadyConfigured {
		t.Fatalf("second setup err = %v, want ErrAuthAlreadyConfigured", err)
	}
	user, hash, configured := svc.AdminCredential()
	if !configured || user != "operator" || hash == "" {
		t.Fatalf("resolved credential = %q/%q/%v, want operator/<hash>/true", user, hash, configured)
	}
}

// TestSetupUsernameDefaultsToAdmin covers the blank-username normalization.
func TestSetupUsernameDefaultsToAdmin(t *testing.T) {
	svc, _ := newAuthTestService(t)
	if err := svc.SetupAdminCredential("  ", "hunter2hunter2"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if user, _, _ := svc.AdminCredential(); user != "admin" {
		t.Fatalf("username = %q, want admin", user)
	}
}

// TestChangeAdminCredential_MigratesTOTPOnRename verifies that an active TOTP
// enrollment is moved to the new username key so 2FA is not silently disabled.
func TestChangeAdminCredential_MigratesTOTPOnRename(t *testing.T) {
	svc, fake := newAuthTestService(t)
	if err := svc.SetupAdminCredential("admin", "hunter2hunter2"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Pre-populate a TOTP row under the current username.
	fake.totpRows["admin"] = db.TOTPState{
		Username:          "admin",
		SecretCipher:      "cipher-payload",
		EnrolledAt:        "2026-01-01T00:00:00Z",
		BackupCodesCipher: "backup-cipher",
	}

	if err := svc.ChangeAdminCredential("alice", "newpassword1"); err != nil {
		t.Fatalf("change: %v", err)
	}

	// Old row must be gone; new row must exist under the new username.
	if _, exists := fake.totpRows["admin"]; exists {
		t.Error("old totp row still present under 'admin' after rename")
	}
	migrated, exists := fake.totpRows["alice"]
	if !exists {
		t.Fatal("totp row not migrated to 'alice'")
	}
	if migrated.Username != "alice" {
		t.Errorf("migrated totp row Username = %q, want 'alice'", migrated.Username)
	}
	if migrated.SecretCipher != "cipher-payload" {
		t.Errorf("migrated totp row SecretCipher = %q, want 'cipher-payload'", migrated.SecretCipher)
	}
}

// TestChangeAdminCredential_NoTOTPMigrationIfNotEnrolled confirms that a rename
// with no active TOTP enrollment succeeds without writing any TOTP row.
func TestChangeAdminCredential_NoTOTPMigrationIfNotEnrolled(t *testing.T) {
	svc, fake := newAuthTestService(t)
	if err := svc.SetupAdminCredential("admin", "hunter2hunter2"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// No TOTP row pre-populated.

	if err := svc.ChangeAdminCredential("alice", "newpassword1"); err != nil {
		t.Fatalf("change: %v", err)
	}

	if len(fake.totpRows) != 0 {
		t.Errorf("expected no totp rows after rename without enrollment, got %v", fake.totpRows)
	}
}

// TestChangeAndClearAdminCredential covers the change + reset round-trip.
func TestChangeAndClearAdminCredential(t *testing.T) {
	svc, _ := newAuthTestService(t)
	if err := svc.SetupAdminCredential("operator", "hunter2hunter2"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := svc.ChangeAdminCredential("renamed", "anotherpassword"); err != nil {
		t.Fatalf("change: %v", err)
	}
	if user, _, _ := svc.AdminCredential(); user != "renamed" {
		t.Fatalf("username after change = %q, want renamed", user)
	}
	if err := svc.ClearAdminCredential(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if svc.IsAuthConfigured() {
		t.Fatalf("expected unconfigured after clear")
	}
}
