package secretbox

import (
	"bytes"
	"strings"
	"testing"
)

func resetKey(t *testing.T) {
	t.Helper()
	keyMu.Lock()
	key = nil
	keyMu.Unlock()
}

func TestSealOpen_RoundTrip(t *testing.T) {
	resetKey(t)
	raw, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	if err := SetKey(raw); err != nil {
		t.Fatalf("SetKey() error = %v", err)
	}

	plaintext := []byte("hunter2-device-password")
	blob, err := Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	if !IsBlob(blob) {
		t.Errorf("Seal() output must be recognizable as a blob, got %q", blob)
	}
	if strings.Contains(blob, "hunter2") {
		t.Errorf("blob leaks plaintext: %q", blob)
	}

	got, err := Open(blob)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("Open() = %q, want %q", got, plaintext)
	}
}

func TestSeal_EmptyInput(t *testing.T) {
	resetKey(t)
	k, _ := GenerateKey()
	_ = SetKey(k)

	blob, err := Seal(nil)
	if err != nil {
		t.Fatalf("Seal(nil) error = %v", err)
	}
	if blob != "" {
		t.Errorf("Seal(nil) = %q, want empty", blob)
	}
	plain, err := Open("")
	if err != nil {
		t.Fatalf("Open(\"\") error = %v", err)
	}
	if len(plain) != 0 {
		t.Errorf("Open(\"\") = %q, want empty", plain)
	}
}

func TestOpen_RejectsWrongKey(t *testing.T) {
	resetKey(t)
	k1, _ := GenerateKey()
	_ = SetKey(k1)
	blob, err := SealString("payload")
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}

	k2, _ := GenerateKey()
	_ = SetKey(k2)
	if _, err := Open(blob); err == nil {
		t.Errorf("Open() with wrong key should fail")
	}
}

func TestOpen_RejectsTamperedCiphertext(t *testing.T) {
	resetKey(t)
	k, _ := GenerateKey()
	_ = SetKey(k)
	blob, _ := SealString("payload")

	// Flip a byte deep inside the ciphertext portion, past the last `$`
	// separator. We avoid the tail of the string because base64 padding bytes
	// (`=`) and the last "data" char of a padded encoding both have don't-care
	// bits that decode to identical bytes when the low bit is flipped — that
	// would be a no-op and the test would flake.
	sep := strings.LastIndex(blob, "$")
	if sep < 0 || sep >= len(blob)-2 {
		t.Fatalf("malformed blob: %q", blob)
	}
	idx := sep + 1 + (len(blob)-sep-1)/2
	tampered := blob[:idx] + string([]byte{blob[idx] ^ 1}) + blob[idx+1:]
	if _, err := Open(tampered); err == nil {
		t.Errorf("Open() of tampered blob should fail")
	}
}

func TestOpen_RejectsUnknownVersion(t *testing.T) {
	resetKey(t)
	k, _ := GenerateKey()
	_ = SetKey(k)
	if _, err := Open("v2$AAAA$BBBB"); err == nil {
		t.Errorf("Open() should reject unknown version prefix")
	}
	if _, err := Open("garbage"); err == nil {
		t.Errorf("Open() should reject malformed blob")
	}
}

func TestSetKey_RejectsWrongLength(t *testing.T) {
	if err := SetKey(make([]byte, 16)); err == nil {
		t.Errorf("SetKey() should reject 16-byte key")
	}
}

func TestSeal_RequiresKey(t *testing.T) {
	resetKey(t)
	if _, err := SealString("payload"); err == nil {
		t.Errorf("Seal() without key should fail")
	}
	if _, err := Open("v1$AAAA$BBBB"); err == nil {
		t.Errorf("Open() without key should fail")
	}
}
