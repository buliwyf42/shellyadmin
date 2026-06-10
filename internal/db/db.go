// Package db is the SQLite persistence layer. This file owns the connection
// lifecycle (Open/Close), schema migrations, the online snapshot, and the
// package-wide helpers. Domain queries live in sibling files, one per table
// family: devices.go, jobs.go, settings.go, templates.go, credentials.go
// (+ admin_credentials.go), auditlog.go, sessions.go, totp.go, pat.go,
// runtimelock.go, backup.go.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"shellyadmin/internal/core/secretbox"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type DB struct {
	sql *sql.DB
}

func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	// modernc.org/sqlite applies _pragma=... entries on every new connection in
	// the pool, which is the only correct place to set per-connection PRAGMAs
	// (busy_timeout, foreign_keys). journal_mode=WAL is global and persists in
	// the file but pinning it here avoids a future "what mode are we in" doubt
	// and matches the prior conn.Exec call's intent.
	dsn := "file:" + filepath.Join(dataDir, "shellyctl.db") +
		"?_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db := &DB{sql: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

// decryptCipher unwraps a non-empty secretbox cipher blob back to plaintext.
// Empty input is a legitimate "secret was empty" case — return as-is rather
// than asking the cipher layer to validate nothing.
func decryptCipher(cipher string) (string, error) {
	if cipher == "" {
		return "", nil
	}
	return secretbox.OpenString(cipher)
}

func (db *DB) Close() error { return db.sql.Close() }

// SnapshotTo writes an atomic, online-safe copy of the running database
// to path via SQLite's `VACUUM INTO` statement. The destination must not
// exist (SQLite refuses to overwrite). Caller is responsible for
// rotation / retention. Used by services.runAutoBackupOnce (S12).
func (db *DB) SnapshotTo(path string) error {
	// VACUUM INTO uses positional-but-string-quoted argument; SQLite's
	// prepared-statement binding does not allow parameter substitution
	// in the VACUUM INTO target. Escape single quotes in the path
	// defensively — operator-controlled values, but a path containing
	// `'` would break the statement otherwise.
	escaped := strings.ReplaceAll(path, "'", "''")
	_, err := db.sql.Exec("VACUUM INTO '" + escaped + "'")
	return err
}

func (db *DB) migrate() error {
	if _, err := db.sql.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return err
	}
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return err
	}
	type migration struct {
		version int
		body    string
	}
	var migrations []migration
	for _, entry := range entries {
		name := entry.Name()
		raw, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		v, _ := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		migrations = append(migrations, migration{version: v, body: string(raw)})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	for _, migration := range migrations {
		var exists int
		_ = db.sql.QueryRow(`SELECT 1 FROM schema_migrations WHERE version = ?`, migration.version).Scan(&exists)
		if exists == 1 {
			continue
		}
		if strings.TrimSpace(migration.body) != "" {
			if _, err := db.sql.Exec(migration.body); err != nil {
				return fmt.Errorf("migration %d failed: %w", migration.version, err)
			}
		}
		if _, err := db.sql.Exec(`INSERT INTO schema_migrations(version) VALUES (?)`, migration.version); err != nil {
			return err
		}
	}
	return nil
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
