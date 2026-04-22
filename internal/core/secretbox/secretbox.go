// Package secretbox provides envelope encryption for at-rest credential
// storage. The package exposes a tiny, stateful API: SetKey() loads a 32-byte
// key exactly once at startup, Seal() encrypts a plaintext, and Open() decrypts
// a blob produced by Seal().
//
// Threat model: this defends against offline database exposure (stolen backup,
// container escape reading /data, misconfigured volume) where an attacker
// obtains shellyctl.db but not the key file. It does not defend against a live
// system compromise where the process memory is readable. See
// docs/SECURITY.md.
//
// Blob format: `v1$<base64-nonce>$<base64-ciphertext>` (URL-safe base64 with
// padding). The version prefix allows future migration without guessing.
package secretbox

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/nacl/secretbox"
)

const (
	keyLen   = 32
	nonceLen = 24
	prefix   = "v1"
)

var (
	keyMu sync.RWMutex
	key   *[keyLen]byte
)

// SetKey installs the encryption key used by Seal/Open. raw must be exactly
// 32 bytes. Calling SetKey with a different key value will replace the
// previous key, which invalidates any existing blobs — callers should not do
// this outside of controlled rotation.
func SetKey(raw []byte) error {
	if len(raw) != keyLen {
		return fmt.Errorf("secretbox: key must be %d bytes, got %d", keyLen, len(raw))
	}
	var dst [keyLen]byte
	copy(dst[:], raw)
	keyMu.Lock()
	key = &dst
	keyMu.Unlock()
	return nil
}

// HasKey reports whether SetKey has been called successfully.
func HasKey() bool {
	keyMu.RLock()
	defer keyMu.RUnlock()
	return key != nil
}

// GenerateKey returns a fresh 32-byte key suitable for SetKey.
func GenerateKey() ([]byte, error) {
	buf := make([]byte, keyLen)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// Seal encrypts plaintext with the installed key and returns a versioned
// blob. An empty plaintext returns an empty string — callers treat "" as
// "no secret stored" and should not round-trip through Seal.
func Seal(plaintext []byte) (string, error) {
	if len(plaintext) == 0 {
		return "", nil
	}
	keyMu.RLock()
	k := key
	keyMu.RUnlock()
	if k == nil {
		return "", errors.New("secretbox: key not initialized")
	}
	var nonce [nonceLen]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	ct := secretbox.Seal(nil, plaintext, &nonce, k)
	enc := base64.StdEncoding
	return prefix + "$" + enc.EncodeToString(nonce[:]) + "$" + enc.EncodeToString(ct), nil
}

// Open decrypts a blob produced by Seal. An empty blob returns an empty
// byte slice, matching the Seal contract.
func Open(blob string) ([]byte, error) {
	if blob == "" {
		return nil, nil
	}
	parts := strings.SplitN(blob, "$", 3)
	if len(parts) != 3 || parts[0] != prefix {
		return nil, errors.New("secretbox: unrecognized blob format")
	}
	keyMu.RLock()
	k := key
	keyMu.RUnlock()
	if k == nil {
		return nil, errors.New("secretbox: key not initialized")
	}
	enc := base64.StdEncoding
	nonceBytes, err := enc.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("secretbox: nonce decode: %w", err)
	}
	if len(nonceBytes) != nonceLen {
		return nil, fmt.Errorf("secretbox: nonce must be %d bytes, got %d", nonceLen, len(nonceBytes))
	}
	var nonce [nonceLen]byte
	copy(nonce[:], nonceBytes)
	ct, err := enc.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("secretbox: ciphertext decode: %w", err)
	}
	plain, ok := secretbox.Open(nil, ct, &nonce, k)
	if !ok {
		return nil, errors.New("secretbox: authentication failed")
	}
	return plain, nil
}

// SealString is a convenience for string payloads.
func SealString(plaintext string) (string, error) {
	return Seal([]byte(plaintext))
}

// OpenString is a convenience for string payloads.
func OpenString(blob string) (string, error) {
	plain, err := Open(blob)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// IsBlob reports whether s looks like a Seal output (has the version prefix).
// Used by migration code to tell plaintext rows from already-encrypted ones.
func IsBlob(s string) bool {
	return strings.HasPrefix(s, prefix+"$")
}
