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

// TestSaveCredentialEncryptsAtRest proves that after SaveCredential the
// on-disk row holds no plaintext secrets — both password and ha1 are only
// reachable through the cipher columns. Migration 020 (v0.1.7) dropped the
// legacy plaintext columns so this is now structurally enforced; the test
// guards against a future regression where an INSERT path forgets to seal.
func TestSaveCredentialEncryptsAtRest(t *testing.T) {
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

	var cipherPassword, cipherHA1 string
	err := database.sql.QueryRow(
		`SELECT password_cipher, ha1_cipher FROM credentials WHERE name = ?`,
		cred.Name,
	).Scan(&cipherPassword, &cipherHA1)
	if err != nil {
		t.Fatalf("SELECT: %v", err)
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
