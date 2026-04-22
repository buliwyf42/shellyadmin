package db

import (
	"testing"

	"shellyadmin/internal/core/secretbox"
	"shellyadmin/internal/models"
)

func ensureSecretboxKey(t *testing.T) {
	t.Helper()
	if secretbox.HasKey() {
		return
	}
	raw, err := secretbox.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if err := secretbox.SetKey(raw); err != nil {
		t.Fatalf("SetKey: %v", err)
	}
}

// TestSaveCredentialLeavesNoPlaintextInSQLite proves that after SaveCredential
// the on-disk row holds no plaintext secrets — both password and ha1 are only
// reachable through the cipher columns.
func TestSaveCredentialLeavesNoPlaintextInSQLite(t *testing.T) {
	ensureSecretboxKey(t)
	database := openTestDB(t)

	cred := models.Credential{
		Name:     "fleet-admin",
		Username: "admin",
		Password: "sekretp@ss-ABCXYZ",
		HA1:      "deadbeefcafef00dfeedface0000ffff",
		Tags:     []string{"prod"},
	}
	if err := database.SaveCredential(cred); err != nil {
		t.Fatalf("SaveCredential: %v", err)
	}

	var plainPassword, plainHA1, cipherPassword, cipherHA1 string
	err := database.sql.QueryRow(
		`SELECT password, ha1, password_cipher, ha1_cipher FROM credentials WHERE name = ?`,
		cred.Name,
	).Scan(&plainPassword, &plainHA1, &cipherPassword, &cipherHA1)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if plainPassword != "" || plainHA1 != "" {
		t.Errorf("plaintext columns must be empty, got password=%q ha1=%q", plainPassword, plainHA1)
	}
	if cipherPassword == "" || cipherHA1 == "" {
		t.Errorf("cipher columns must be populated, got password_cipher=%q ha1_cipher=%q", cipherPassword, cipherHA1)
	}
	if !secretbox.IsBlob(cipherPassword) || !secretbox.IsBlob(cipherHA1) {
		t.Errorf("cipher columns must look like v1 blobs")
	}

	got, err := database.GetCredential(cred.Name)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if got.Password != cred.Password || got.HA1 != cred.HA1 {
		t.Errorf("round-trip mismatch: password=%q ha1=%q", got.Password, got.HA1)
	}
}

// TestEncryptPlaintextCredentialsSweep simulates a pre-upgrade database by
// writing plaintext directly, then asserts Open()'s sweep migrates the rows.
func TestEncryptPlaintextCredentialsSweep(t *testing.T) {
	ensureSecretboxKey(t)
	database := openTestDB(t)

	if _, err := database.sql.Exec(`
		INSERT INTO credentials(name, username, password, ha1, password_cipher, ha1_cipher, tags, created_at, updated_at)
		VALUES ('legacy', 'admin', 'plain-pass', 'plain-ha1', '', '', '[]', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	if err := database.encryptPlaintextCredentials(); err != nil {
		t.Fatalf("encryptPlaintextCredentials: %v", err)
	}

	var plainPassword, plainHA1, cipherPassword, cipherHA1 string
	if err := database.sql.QueryRow(
		`SELECT password, ha1, password_cipher, ha1_cipher FROM credentials WHERE name = 'legacy'`,
	).Scan(&plainPassword, &plainHA1, &cipherPassword, &cipherHA1); err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if plainPassword != "" || plainHA1 != "" {
		t.Errorf("plaintext should be cleared after sweep, got password=%q ha1=%q", plainPassword, plainHA1)
	}
	if !secretbox.IsBlob(cipherPassword) || !secretbox.IsBlob(cipherHA1) {
		t.Errorf("cipher columns should be populated, got password_cipher=%q ha1_cipher=%q", cipherPassword, cipherHA1)
	}

	got, err := database.GetCredential("legacy")
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if got.Password != "plain-pass" || got.HA1 != "plain-ha1" {
		t.Errorf("sweep must preserve values, got password=%q ha1=%q", got.Password, got.HA1)
	}
}
