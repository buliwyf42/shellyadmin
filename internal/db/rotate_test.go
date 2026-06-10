package db

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"shellyadmin/internal/core/secretbox"
)

// Fixed keys + explicit-key seal/open so this test never touches the
// process-global secretbox key other package tests rely on.
var (
	rotateKeyA = bytes.Repeat([]byte{0xA1}, 32)
	rotateKeyB = bytes.Repeat([]byte{0xB2}, 32)
)

func sealWithKey(t *testing.T, key []byte, plain string) string {
	t.Helper()
	out, err := secretbox.SealStringWithKey(key, plain)
	if err != nil {
		t.Fatalf("SealStringWithKey: %v", err)
	}
	return out
}

func openA(cipher string) (string, error) { return secretbox.OpenStringWithKey(rotateKeyA, cipher) }
func sealB(plain string) (string, error)  { return secretbox.SealStringWithKey(rotateKeyB, plain) }

// seedSealedFixtures writes one row per sealed surface, all under key A.
// The settings row carries an extra field to prove rotation round-trips the
// JSON without dropping anything it doesn't know about.
func seedSealedFixtures(t *testing.T, database *DB) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := database.sql.Exec(query, args...); err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
	}
	exec(`INSERT INTO credentials(name, username, password_cipher, ha1_cipher, tags, created_at, updated_at)
		VALUES ('cred-1', 'admin', ?, ?, '[]', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		sealWithKey(t, rotateKeyA, "cred-pass"), sealWithKey(t, rotateKeyA, "cred-ha1"))
	// Empty ha1 cipher must survive rotation untouched.
	exec(`INSERT INTO credential_groups(name, credential_ref, password_cipher, ha1_cipher, tags, created_at, updated_at)
		VALUES ('group-1', 'group-1', ?, '', '[]', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		sealWithKey(t, rotateKeyA, "group-pass"))
	exec(`INSERT INTO totp_state(username, secret_cipher, enrolled_at, last_verified_at, backup_codes_cipher, backup_codes_used)
		VALUES ('admin', ?, '2026-01-01T00:00:00Z', '', ?, 0)`,
		sealWithKey(t, rotateKeyA, "totp-secret"), sealWithKey(t, rotateKeyA, `["code1","code2"]`))
	settings := map[string]any{
		"mcp_token": sealWithKey(t, rotateKeyA, "mcp-plain-token-0123"),
		"subnets":   []string{"10.0.0.0/24"},
	}
	body, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	exec(`INSERT INTO settings(key, value) VALUES ('app', ?)`, string(body))
}

func assertOpensWith(t *testing.T, key []byte, cipher, want, label string) {
	t.Helper()
	got, err := secretbox.OpenStringWithKey(key, cipher)
	if err != nil {
		t.Fatalf("%s: open failed: %v", label, err)
	}
	if got != want {
		t.Fatalf("%s: got %q, want %q", label, got, want)
	}
}

func TestRotateSealedColumnsDryRunWritesNothing(t *testing.T) {
	database := openTestDB(t)
	seedSealedFixtures(t, database)

	report, err := database.RotateSealedColumns(openA, sealB, false)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if report.Credentials != 1 || report.CredentialGroups != 1 || report.TOTPUsers != 1 || !report.MCPToken {
		t.Fatalf("dry-run report = %+v, want 1/1/1/true", report)
	}

	// Everything must still open with the OLD key.
	var cipher string
	if err := database.sql.QueryRow(`SELECT password_cipher FROM credentials WHERE name='cred-1'`).Scan(&cipher); err != nil {
		t.Fatalf("select: %v", err)
	}
	assertOpensWith(t, rotateKeyA, cipher, "cred-pass", "credential password after dry run")
}

func TestRotateSealedColumnsApply(t *testing.T) {
	database := openTestDB(t)
	seedSealedFixtures(t, database)

	report, err := database.RotateSealedColumns(openA, sealB, true)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if report.Credentials != 1 || report.CredentialGroups != 1 || report.TOTPUsers != 1 || !report.MCPToken {
		t.Fatalf("report = %+v, want 1/1/1/true", report)
	}

	var passwordCipher, ha1Cipher string
	if err := database.sql.QueryRow(`SELECT password_cipher, ha1_cipher FROM credentials WHERE name='cred-1'`).Scan(&passwordCipher, &ha1Cipher); err != nil {
		t.Fatalf("select credential: %v", err)
	}
	assertOpensWith(t, rotateKeyB, passwordCipher, "cred-pass", "credential password")
	assertOpensWith(t, rotateKeyB, ha1Cipher, "cred-ha1", "credential ha1")
	if _, err := secretbox.OpenStringWithKey(rotateKeyA, passwordCipher); err == nil {
		t.Fatal("old key still opens the rotated credential password")
	}

	var groupPassword, groupHA1 string
	if err := database.sql.QueryRow(`SELECT password_cipher, ha1_cipher FROM credential_groups WHERE name='group-1'`).Scan(&groupPassword, &groupHA1); err != nil {
		t.Fatalf("select group: %v", err)
	}
	assertOpensWith(t, rotateKeyB, groupPassword, "group-pass", "group password")
	if groupHA1 != "" {
		t.Fatalf("empty ha1 cipher must stay empty, got %q", groupHA1)
	}

	var totpSecret, totpBackup string
	if err := database.sql.QueryRow(`SELECT secret_cipher, backup_codes_cipher FROM totp_state WHERE username='admin'`).Scan(&totpSecret, &totpBackup); err != nil {
		t.Fatalf("select totp: %v", err)
	}
	assertOpensWith(t, rotateKeyB, totpSecret, "totp-secret", "totp secret")
	assertOpensWith(t, rotateKeyB, totpBackup, `["code1","code2"]`, "totp backup codes")

	var raw string
	if err := database.sql.QueryRow(`SELECT value FROM settings WHERE key='app'`).Scan(&raw); err != nil {
		t.Fatalf("select settings: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	token, _ := settings["mcp_token"].(string)
	assertOpensWith(t, rotateKeyB, token, "mcp-plain-token-0123", "mcp token")
	// The unrelated field must survive the JSON round-trip.
	if _, ok := settings["subnets"]; !ok {
		t.Fatal("rotation dropped an unrelated settings field (subnets)")
	}
}

func TestRotateSealedColumnsWrongKeyRollsBack(t *testing.T) {
	database := openTestDB(t)
	seedSealedFixtures(t, database)

	// Key B never sealed anything yet — opening with it must fail, and the
	// failure must leave every row untouched (single-transaction rollback).
	openWrong := func(cipher string) (string, error) {
		return secretbox.OpenStringWithKey(rotateKeyB, cipher)
	}
	sealA := func(plain string) (string, error) {
		return secretbox.SealStringWithKey(rotateKeyA, plain)
	}
	_, err := database.RotateSealedColumns(openWrong, sealA, true)
	if err == nil {
		t.Fatal("expected rotation with wrong key to fail")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("expected secretbox auth failure, got: %v", err)
	}

	var cipher string
	if err := database.sql.QueryRow(`SELECT password_cipher FROM credentials WHERE name='cred-1'`).Scan(&cipher); err != nil {
		t.Fatalf("select: %v", err)
	}
	assertOpensWith(t, rotateKeyA, cipher, "cred-pass", "credential password after failed rotation")
}
