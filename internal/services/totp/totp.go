// Package totp implements RFC 6238 (TOTP) + backup-code generation
// against the standard library. No external crypto dependency — the
// implementation is short enough (HMAC-SHA1 + dynamic truncation per the
// RFC's appendix B reference) that the supply-chain-risk of pulling in
// pquerna/otp or xlzd/gotp outweighs the LOC saved.
//
// T1 from the consolidated review (docs/plans/phase-4c-auth-strategics.md,
// Block 4c.1). v0.3.0.
//
// Higher-level orchestration (enrollment flow, login integration,
// secretbox-sealing the persisted secret) lives in the caller; this
// package is intentionally limited to the primitive crypto operations.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// SecretBytes is the secret's entropy in raw bytes. RFC 6238 §5.1
	// requires at least 128 bits; we go to 160 (the standard HMAC-SHA1
	// block size) so the secret fully uses the hash's internal state.
	SecretBytes = 20

	// Period is the TOTP rolling window in seconds. RFC 6238 §5.2
	// recommends 30s; every popular authenticator (Google Authenticator,
	// 1Password, Authy, ...) defaults to this value.
	Period = 30

	// Digits is the displayed code length. The RFC permits 6, 7, or 8;
	// 6 is universally supported.
	Digits = 6

	// SkewWindow is the count of adjacent periods Verify accepts on
	// either side of the current period. RFC 6238 §5.2 recommends 1
	// (i.e. accept t-30, t, t+30) to cover clock skew + transit latency.
	SkewWindow = 1

	// BackupCodes is the number of single-use recovery codes minted at
	// enrolment. 10 follows the pattern operators see in GitHub /
	// Google account flows.
	BackupCodes = 10

	// BackupCodeBytes is the raw entropy per backup code. 5 bytes →
	// 8-char base32 ≈ 40 bits, the same security budget GitHub uses
	// for its 16-char hex codes (64 bits) once you account for the
	// 10-code budget per user (so brute-forcing ANY of the 10 needs
	// 2^36 attempts amortised — well past credible offline-cracking).
	BackupCodeBytes = 5
)

// GenerateSecret returns a fresh base32-encoded TOTP secret suitable for
// pasting into a QR code or hand-keying into an authenticator app. The
// output strips the trailing `=` padding RFC 4648 mandates, matching
// the "GA-friendly" format every other TOTP impl produces.
func GenerateSecret() (string, error) {
	raw := make([]byte, SecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("totp: read random bytes: %w", err)
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "="), nil
}

// Generate computes the TOTP code for `secret` at the given wall-clock
// time. Exposed so tests can pin the time; callers in production should
// use Verify which handles the time + skew window itself.
func Generate(secret string, at time.Time) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return "", err
	}
	counter := uint64(at.Unix() / Period)
	return computeCode(key, counter), nil
}

// Verify returns true when `code` matches the secret's TOTP code at
// `at` or at any of the ±SkewWindow adjacent periods. Constant-time
// comparison protects against timing-side-channel attacks (overkill
// against a TOTP brute-force given the ~30s lifetime + per-IP rate
// limiting, but cheap enough that there's no reason not to).
func Verify(secret, code string, at time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != Digits {
		return false
	}
	key, err := decodeSecret(secret)
	if err != nil {
		return false
	}
	base := uint64(at.Unix() / Period)
	for delta := -SkewWindow; delta <= SkewWindow; delta++ {
		counter := base + uint64(int64(delta))
		// Guard against underflow when at.Unix() is near 0 and delta is
		// negative — production never sees Unix epoch, but a fuzz test
		// or a clock-misconfigured CI might.
		if delta < 0 && base < uint64(-delta) {
			continue
		}
		candidate := computeCode(key, counter)
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// OTPAuthURI builds the `otpauth://` URI that every authenticator app
// reads from a QR code (RFC 6238 §6 + the de-facto schema Google's
// authenticator-format docs ratified). issuer + account become the
// human-readable label; secret is the same base32 string GenerateSecret
// returns.
func OTPAuthURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", Digits))
	q.Set("period", fmt.Sprintf("%d", Period))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// GenerateBackupCodes returns `n` freshly-randomised codes formatted as
// `XXXX-XXXX` (an 8-char base32 string with a dash in the middle for
// readability when the operator hand-copies them into a password
// manager). The codes are NOT hashed — the caller is expected to call
// HashBackupCode on each before persisting.
func GenerateBackupCodes(n int) ([]string, error) {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		raw := make([]byte, BackupCodeBytes)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("totp: read random bytes: %w", err)
		}
		enc := strings.ToUpper(strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "="))
		out[i] = enc[:4] + "-" + enc[4:]
	}
	return out, nil
}

// HashBackupCode returns the hex-encoded SHA-256 of a normalised
// backup code. Normalisation strips the dash + uppercases — so the
// operator can paste "abcd-efgh" or "ABCDEFGH" and both verify.
//
// SHA-256 (not argon2) is sufficient here because the persisted column
// is ALSO secretbox-sealed (defense in depth: the attacker needs the
// encryption key to even reach the hashes, and then needs ~2^40
// attempts to brute-force any single 8-char code). If the encryption
// key is in operator's filesystem, switching to argon2 buys nothing.
func HashBackupCode(code string) string {
	normalised := normaliseBackupCode(code)
	sum := sha256.Sum256([]byte(normalised))
	return hex.EncodeToString(sum[:])
}

// VerifyBackupCode finds `code` in the hashed list and returns the
// matched index, or -1 + false on miss. Caller is responsible for
// marking the matched index as used in storage (the hash list itself
// is immutable; storage tracks a separate usedMask). Comparison is
// constant-time against every entry to avoid timing oracles.
func VerifyBackupCode(hashes []string, code string) (int, bool) {
	want := HashBackupCode(code)
	matchedIndex := -1
	for i, h := range hashes {
		if subtle.ConstantTimeCompare([]byte(h), []byte(want)) == 1 {
			matchedIndex = i
		}
	}
	return matchedIndex, matchedIndex >= 0
}

// EncodeBackupHashes serialises the hash list to JSON so the caller can
// secretbox-seal it. The shape is `["hexhash", "hexhash", ...]` — no
// metadata; the per-code "used" bit lives in the row's
// backup_codes_used bitmask alongside.
func EncodeBackupHashes(hashes []string) ([]byte, error) {
	return json.Marshal(hashes)
}

// DecodeBackupHashes is the inverse — caller secretbox-opens the row
// then feeds the bytes here.
func DecodeBackupHashes(blob []byte) ([]string, error) {
	var out []string
	if err := json.Unmarshal(blob, &out); err != nil {
		return nil, fmt.Errorf("totp: decode backup hashes: %w", err)
	}
	return out, nil
}

// --- internals ---

// decodeSecret accepts the un-padded base32 GenerateSecret produces or
// the padded form any authenticator app might paste back. Spaces are
// stripped because some operators copy-paste with formatting.
func decodeSecret(secret string) ([]byte, error) {
	cleaned := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	// base32 stdlib requires the padding; restore it.
	if pad := len(cleaned) % 8; pad != 0 {
		cleaned += strings.Repeat("=", 8-pad)
	}
	return base32.StdEncoding.DecodeString(cleaned)
}

// computeCode is the RFC 6238 §5.3 algorithm (which is RFC 4226 §5.3
// HOTP underneath, with the counter derived from time). Implementation
// matches the appendix-B reference verbatim.
func computeCode(key []byte, counter uint64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)
	// Dynamic truncation: bottom nibble of the last byte gives the
	// offset, then take 4 bytes starting there + mask high bit.
	offset := sum[len(sum)-1] & 0x0F
	value := (uint32(sum[offset])&0x7F)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	mod := uint32(1)
	for i := 0; i < Digits; i++ {
		mod *= 10
	}
	value %= mod
	return fmt.Sprintf("%0*d", Digits, value)
}

func normaliseBackupCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(code), "-", ""), " ", ""))
}
