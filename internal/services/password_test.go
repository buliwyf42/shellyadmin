package services

import (
	"strings"
	"testing"
)

func TestHashPassword_RoundTrip(t *testing.T) {
	phc, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !IsPasswordHash(phc) {
		t.Errorf("expected PHC format, got %q", phc)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=") {
		t.Errorf("unexpected PHC prefix: %q", phc)
	}

	ok, err := VerifyPassword("correct-horse-battery-staple", phc)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Errorf("VerifyPassword() = false, want true for matching password")
	}
}

func TestVerifyPassword_RejectsWrongPassword(t *testing.T) {
	phc, _ := HashPassword("right-password")
	ok, err := VerifyPassword("wrong-password", phc)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if ok {
		t.Errorf("VerifyPassword() = true, want false")
	}
}

func TestHashPassword_ProducesUniqueSalts(t *testing.T) {
	a, _ := HashPassword("same-input")
	b, _ := HashPassword("same-input")
	if a == b {
		t.Errorf("two hashes of the same password should differ (distinct salts)")
	}
}

func TestVerifyPassword_RejectsBadFormat(t *testing.T) {
	if _, err := VerifyPassword("x", "plaintext-not-a-hash"); err == nil {
		t.Errorf("expected error on non-PHC input")
	}
	if _, err := VerifyPassword("x", "$argon2i$v=19$m=1,t=1,p=1$aaaa$bbbb"); err == nil {
		t.Errorf("expected error on non-argon2id variant")
	}
}

func TestHashPassword_RejectsEmpty(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Errorf("empty password must not hash")
	}
}

// TestIsLegacyParameters locks T6's behaviour: hashes produced with the
// current defaults are NOT legacy; a hand-crafted hash with the v0.2.x
// m=64MiB parameter IS legacy. Non-argon2id strings return false (the
// helper is "is the parameter set below the floor", not a general
// validity check).
func TestIsLegacyParameters(t *testing.T) {
	current, err := HashPassword("test-current")
	if err != nil {
		t.Fatalf("HashPassword error = %v", err)
	}
	if IsLegacyParameters(current) {
		t.Errorf("freshly hashed password reported as legacy: %s", current)
	}
	legacy := "$argon2id$v=19$m=65536,t=2,p=1$YWFhYWFhYWFhYWFhYWFhYQ$YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE"
	if !IsLegacyParameters(legacy) {
		t.Errorf("v0.2.x-shaped m=64MiB hash should be flagged as legacy: %s", legacy)
	}
	if IsLegacyParameters("not a phc string") {
		t.Errorf("non-PHC input should not be flagged as legacy")
	}
	if IsLegacyParameters("$bcrypt$2y$10$ldsmaSGDSGklfsfkl") {
		t.Errorf("non-argon2id PHC should not be flagged as legacy")
	}
}
