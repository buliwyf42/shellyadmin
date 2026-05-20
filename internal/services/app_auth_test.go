package services

import (
	"context"
	"testing"
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
