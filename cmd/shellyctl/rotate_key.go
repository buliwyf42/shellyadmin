package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"shellyadmin/internal/core/secretbox"
	"shellyadmin/internal/db"
	"shellyadmin/internal/services"
	"shellyadmin/internal/services/runtimelock"
)

// runRotateKey is the `shellyctl rotate-key` subcommand (review item 8). It
// re-encrypts every secretbox-sealed value in the database — device
// credentials, credential groups, TOTP material, the persisted MCP token —
// from the current key to a new one, in a single transaction. This replaces
// the manual clear-everything rotation playbook in docs/SECURITY.md.
//
// Keys come from the environment, never from argv (argv leaks into shell
// history and `ps`):
//
//	SHELLYADMIN_ENCRYPTION_KEY[_FILE]      — the CURRENT key (same vars the server uses)
//	SHELLYADMIN_NEW_ENCRYPTION_KEY[_FILE]  — the key to rotate TO (base64, 32 bytes)
//
// Without --force it is a dry run: every blob is opened with the old key
// (catching a wrong key before anything is written) and the would-be counts
// are printed. With --force a timestamped DB backup is written first, then
// the rotation commits. The server must be stopped: a fresh runtime-lock
// heartbeat aborts the run.
func runRotateKey(args []string) {
	force := false
	for _, arg := range args {
		switch arg {
		case "--force", "-f":
			force = true
		default:
			fmt.Fprintf(os.Stderr, "unknown arg: %q\n", arg)
			fmt.Fprintln(os.Stderr, "usage: shellyctl rotate-key [--force]")
			os.Exit(2)
		}
	}

	oldKey, err := decodeKeyEnv("SHELLYADMIN_ENCRYPTION_KEY")
	if err != nil {
		fmt.Fprintln(os.Stderr, "current key:", err)
		os.Exit(2)
	}
	newKey, err := decodeKeyEnv("SHELLYADMIN_NEW_ENCRYPTION_KEY")
	if err != nil {
		fmt.Fprintln(os.Stderr, "new key:", err)
		fmt.Fprintln(os.Stderr, "Generate one with: openssl rand -base64 32")
		os.Exit(2)
	}
	if bytes.Equal(oldKey, newKey) {
		fmt.Fprintln(os.Stderr, "new key is identical to the current key — nothing to rotate")
		os.Exit(2)
	}

	dataDir := getenv("DATA_DIR", "./data")
	database, err := db.Open(dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database open:", err)
		os.Exit(1)
	}
	defer database.Close()

	// A live server would keep writing rows sealed under the old key while
	// we rotate underneath it. A fresh heartbeat on the runtime lock means
	// exactly that — refuse. A stale row (crashed container) is fine.
	if row, err := database.GetRuntimeLock(runtimelock.PrimaryKey); err == nil {
		if acquiredAt, perr := time.Parse(time.RFC3339, row.AcquiredAt); perr == nil {
			if time.Since(acquiredAt) < runtimelock.StaleAfter {
				fmt.Fprintf(os.Stderr, "a server instance appears to be running (lock held by %s pid=%d, heartbeat %s)\n",
					row.Hostname, row.PID, row.AcquiredAt)
				fmt.Fprintln(os.Stderr, "Stop the container first; rotating under a live server would corrupt new writes.")
				os.Exit(1)
			}
		}
	}

	openFn := func(cipher string) (string, error) {
		return secretbox.OpenStringWithKey(oldKey, cipher)
	}
	sealFn := func(plain string) (string, error) {
		return secretbox.SealStringWithKey(newKey, plain)
	}

	if !force {
		report, err := database.RotateSealedColumns(openFn, sealFn, false)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dry run failed:", err)
			fmt.Fprintln(os.Stderr, "Nothing was changed. Is SHELLYADMIN_ENCRYPTION_KEY the key the database was sealed with?")
			os.Exit(1)
		}
		printRotationReport("DRY RUN — nothing written", report)
		fmt.Println("\nRe-run with --force to rotate. A timestamped DB backup is written first.")
		return
	}

	backupPath, err := db.BackupDatabaseFile(dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pre-rotation backup:", err)
		os.Exit(1)
	}
	fmt.Println("pre-rotation backup:", backupPath)

	report, err := database.RotateSealedColumns(openFn, sealFn, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rotation failed (rolled back, database unchanged):", err)
		os.Exit(1)
	}
	printRotationReport("rotated", report)
	fmt.Println("\nNEXT STEP: point SHELLYADMIN_ENCRYPTION_KEY (or _FILE) at the NEW key before")
	fmt.Println("starting the server — the old key no longer opens anything in this database.")
	fmt.Println("Keep the old key and the backup until a refresh against an auth-protected")
	fmt.Println("device succeeds, then retire both.")
}

func printRotationReport(label string, r db.RotationReport) {
	fmt.Printf("%s: %d credentials, %d credential groups, %d TOTP enrollments, mcp token: %v\n",
		label, r.Credentials, r.CredentialGroups, r.TOTPUsers, r.MCPToken)
}

// decodeKeyEnv reads a base64-encoded 32-byte key from envKey (honouring the
// _FILE indirection via services.DecodeSecretValue) and decodes it.
func decodeKeyEnv(envKey string) ([]byte, error) {
	raw := services.DecodeSecretValue(envKey)
	if raw == "" {
		return nil, fmt.Errorf("%s (or %s_FILE) is not set", envKey, envKey)
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid base64: %w", envKey, err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%s must decode to 32 bytes, got %d", envKey, len(key))
	}
	return key, nil
}
