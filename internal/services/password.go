package services

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. Values are moderate: suitable for a low-login-volume
// admin surface without making startup password hashing painful. Tuned per
// OWASP's 2023 recommendation for argon2id (m=64MiB, t=2, p=1) and
// constant-time verified.
const (
	argonTime    uint32 = 2
	argonMemory  uint32 = 64 * 1024 // KiB
	argonThreads uint8  = 1
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

// HashPassword produces a PHC-formatted argon2id string from the supplied
// plaintext. The output can be stored as-is and round-tripped through
// VerifyPassword.
func HashPassword(plain string) (string, error) {
	if plain == "" {
		return "", errors.New("password cannot be empty")
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(plain), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyPassword returns true when plain hashes to phc using the same
// parameters encoded in the PHC string. Comparisons are constant-time.
// phc must start with "$argon2id$" — anything else is treated as an error
// rather than returning false so callers can tell bad input apart from bad
// password.
func VerifyPassword(plain, phc string) (bool, error) {
	parts := strings.Split(phc, "$")
	// "$argon2id$v=…$m=…,t=…,p=…$<salt>$<hash>" splits to 6 fields (the
	// leading "$" produces an empty first segment).
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("unsupported password hash format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("parse version: %w", err)
	}
	if version != argon2.Version {
		return false, fmt.Errorf("unsupported argon2 version %d", version)
	}
	m, t, p, err := parseArgonParams(parts[3])
	if err != nil {
		return false, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode salt: %w", err)
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decode hash: %w", err)
	}
	got := argon2.IDKey([]byte(plain), salt, t, m, p, uint32(len(expected)))
	return subtle.ConstantTimeCompare(got, expected) == 1, nil
}

func parseArgonParams(s string) (memory, time uint32, threads uint8, err error) {
	for _, kv := range strings.Split(s, ",") {
		bits := strings.SplitN(kv, "=", 2)
		if len(bits) != 2 {
			return 0, 0, 0, fmt.Errorf("bad param %q", kv)
		}
		v, parseErr := strconv.ParseUint(bits[1], 10, 32)
		if parseErr != nil {
			return 0, 0, 0, fmt.Errorf("parse %s: %w", bits[0], parseErr)
		}
		switch bits[0] {
		case "m":
			memory = uint32(v)
		case "t":
			time = uint32(v)
		case "p":
			threads = uint8(v)
		default:
			return 0, 0, 0, fmt.Errorf("unknown argon2 param %q", bits[0])
		}
	}
	return memory, time, threads, nil
}

// IsPasswordHash reports whether s is a PHC-formatted argon2id string.
func IsPasswordHash(s string) bool {
	return strings.HasPrefix(s, "$argon2id$")
}
