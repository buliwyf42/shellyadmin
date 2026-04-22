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
