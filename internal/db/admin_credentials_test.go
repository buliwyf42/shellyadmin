package db

import "testing"

func TestAdminCredentialRoundTrip(t *testing.T) {
	database, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	// Fresh DB: no credential.
	if _, _, ok, err := database.GetAdminCredential(); err != nil || ok {
		t.Fatalf("fresh GetAdminCredential = (_, _, %v, %v), want (false, nil)", ok, err)
	}

	if err := database.SaveAdminCredential("operator", "$argon2id$hash1"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	user, hash, ok, err := database.GetAdminCredential()
	if err != nil || !ok || user != "operator" || hash != "$argon2id$hash1" {
		t.Fatalf("Get after save = (%q, %q, %v, %v), want (operator, $argon2id$hash1, true, nil)", user, hash, ok, err)
	}

	// Upsert overwrites the single row rather than inserting a second.
	if err := database.SaveAdminCredential("renamed", "$argon2id$hash2"); err != nil {
		t.Fatalf("Save (update): %v", err)
	}
	user, hash, _, _ = database.GetAdminCredential()
	if user != "renamed" || hash != "$argon2id$hash2" {
		t.Fatalf("Get after update = (%q, %q), want (renamed, $argon2id$hash2)", user, hash)
	}

	if err := database.ClearAdminCredential(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, _, ok, _ := database.GetAdminCredential(); ok {
		t.Fatalf("Get after clear: ok=true, want false")
	}
}
