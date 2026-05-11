package totp

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// TestRFC6238Vectors pins the impl against RFC 6238 Appendix B's
// reference test vectors. The RFC's seed is the ASCII bytes
// "12345678901234567890" (HMAC-SHA1 case). If this test ever fails,
// `computeCode` has drifted from the canonical algorithm.
func TestRFC6238Vectors(t *testing.T) {
	// RFC's seed → un-padded base32 (no whitespace).
	rfcSecret := strings.TrimRight(
		base32.StdEncoding.EncodeToString([]byte("12345678901234567890")),
		"=",
	)
	// Appendix B vectors. Time is seconds since epoch; the RFC uses
	// 8-digit codes but our default is 6 so we truncate to the last 6.
	cases := []struct {
		unix int64
		want string
	}{
		// time: 59           → T=00000001 → 8-digit 94287082 → 6-digit 287082
		{59, "287082"},
		// time: 1111111109   → T=023523EC → 8-digit 07081804 → 6-digit 081804
		{1111111109, "081804"},
		// time: 1111111111   → T=023523ED → 8-digit 14050471 → 6-digit 050471
		{1111111111, "050471"},
		// time: 1234567890   → T=0273EF07 → 8-digit 89005924 → 6-digit 005924
		{1234567890, "005924"},
		// time: 2000000000   → T=03F940AA → 8-digit 69279037 → 6-digit 279037
		{2000000000, "279037"},
	}
	for _, c := range cases {
		got, err := Generate(rfcSecret, time.Unix(c.unix, 0))
		if err != nil {
			t.Fatalf("Generate(t=%d): %v", c.unix, err)
		}
		if got != c.want {
			t.Errorf("Generate(t=%d) = %q, want %q", c.unix, got, c.want)
		}
	}
}

func TestGenerateSecretRoundTrip(t *testing.T) {
	for i := 0; i < 5; i++ {
		secret, err := GenerateSecret()
		if err != nil {
			t.Fatalf("GenerateSecret: %v", err)
		}
		if len(secret) < 32 {
			t.Errorf("secret too short: %q (%d chars)", secret, len(secret))
		}
		if strings.Contains(secret, "=") {
			t.Errorf("secret has trailing padding: %q", secret)
		}
		// Generate + Verify with the SAME timestamp must round-trip.
		now := time.Now()
		code, err := Generate(secret, now)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if !Verify(secret, code, now) {
			t.Errorf("Verify rejected its own freshly-generated code")
		}
	}
}

func TestVerifySkewWindow(t *testing.T) {
	secret, _ := GenerateSecret()
	t0 := time.Unix(1700000000, 0) // arbitrary epoch reference

	codeAtT0, _ := Generate(secret, t0)

	// Within ±Period window: must accept.
	for _, delta := range []time.Duration{0, Period * time.Second, -Period * time.Second} {
		at := t0.Add(delta)
		if !Verify(secret, codeAtT0, at) {
			t.Errorf("Verify rejected code at delta=%v (should accept ±%ds)", delta, Period)
		}
	}

	// Outside the window: must reject. 2× Period away from generation
	// time guarantees the counter has changed by more than SkewWindow=1.
	if Verify(secret, codeAtT0, t0.Add(2*Period*time.Second+1)) {
		t.Errorf("Verify accepted code 2*Period+1 seconds late — skew window leaked")
	}
	if Verify(secret, codeAtT0, t0.Add(-2*Period*time.Second-1)) {
		t.Errorf("Verify accepted code 2*Period+1 seconds early — skew window leaked")
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	secret, _ := GenerateSecret()
	now := time.Now()
	cases := []string{
		"",        // empty
		"12345",   // too short
		"1234567", // too long
		"abcdef",  // non-digits
		"000000",  // numeric but wrong-counter (chance of collision is 10^-6, negligible)
	}
	for _, code := range cases {
		if Verify(secret, code, now) && code != "000000" {
			t.Errorf("Verify accepted malformed code %q", code)
		}
	}
	if Verify(secret, "12345", now) {
		t.Errorf("Verify accepted 5-digit code")
	}
}

func TestOTPAuthURIShape(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP" // "Hello!" — canonical RFC sample
	uri := OTPAuthURI("ShellyAdmin", "admin", secret)
	for _, must := range []string{
		"otpauth://totp/",
		"ShellyAdmin:admin",
		"secret=JBSWY3DPEHPK3PXP",
		"issuer=ShellyAdmin",
		"algorithm=SHA1",
		"digits=6",
		"period=30",
	} {
		if !strings.Contains(uri, must) {
			t.Errorf("OTPAuthURI missing %q\n  got: %s", must, uri)
		}
	}
}

func TestBackupCodesShape(t *testing.T) {
	codes, err := GenerateBackupCodes(BackupCodes)
	if err != nil {
		t.Fatalf("GenerateBackupCodes: %v", err)
	}
	if len(codes) != BackupCodes {
		t.Fatalf("want %d codes, got %d", BackupCodes, len(codes))
	}
	seen := map[string]bool{}
	for _, c := range codes {
		// XXXX-XXXX, all uppercase base32.
		if len(c) != 9 || c[4] != '-' {
			t.Errorf("backup code has wrong shape: %q", c)
		}
		if strings.ToUpper(c) != c {
			t.Errorf("backup code not uppercase: %q", c)
		}
		if seen[c] {
			t.Errorf("backup code collision: %q", c)
		}
		seen[c] = true
	}
}

func TestBackupCodeVerifyAndNormalise(t *testing.T) {
	codes, _ := GenerateBackupCodes(3)
	hashes := make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = HashBackupCode(c)
	}

	for i, c := range codes {
		// Exact match.
		idx, ok := VerifyBackupCode(hashes, c)
		if !ok || idx != i {
			t.Errorf("VerifyBackupCode(%q) = (%d, %v), want (%d, true)", c, idx, ok, i)
		}
		// Lowercase + no dash form must still match.
		ldash := strings.ToLower(strings.ReplaceAll(c, "-", ""))
		idx, ok = VerifyBackupCode(hashes, ldash)
		if !ok || idx != i {
			t.Errorf("VerifyBackupCode(%q normalised) = (%d, %v), want (%d, true)", ldash, idx, ok, i)
		}
		// Leading/trailing spaces must not break it.
		idx, ok = VerifyBackupCode(hashes, "  "+c+" ")
		if !ok || idx != i {
			t.Errorf("VerifyBackupCode(padded %q) = (%d, %v), want (%d, true)", c, idx, ok, i)
		}
	}

	// Wrong code → miss.
	idx, ok := VerifyBackupCode(hashes, "WRNG-CODE")
	if ok || idx != -1 {
		t.Errorf("VerifyBackupCode(wrong) = (%d, %v), want (-1, false)", idx, ok)
	}
}

func TestEncodeDecodeBackupHashes(t *testing.T) {
	codes, _ := GenerateBackupCodes(5)
	hashes := make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = HashBackupCode(c)
	}
	blob, err := EncodeBackupHashes(hashes)
	if err != nil {
		t.Fatalf("EncodeBackupHashes: %v", err)
	}
	got, err := DecodeBackupHashes(blob)
	if err != nil {
		t.Fatalf("DecodeBackupHashes: %v", err)
	}
	if len(got) != len(hashes) {
		t.Fatalf("round-trip lost rows: got %d, want %d", len(got), len(hashes))
	}
	for i := range hashes {
		if got[i] != hashes[i] {
			t.Errorf("round-trip hash[%d] mismatch: got %q, want %q", i, got[i], hashes[i])
		}
	}
}
