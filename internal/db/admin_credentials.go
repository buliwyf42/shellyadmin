package db

import "database/sql"

// GetAdminCredential returns the persisted operator login. ok is false when
// no row exists yet (the "not configured — boot into setup mode" state). The
// pass_hash is an argon2id PHC string, returned as stored.
func (db *DB) GetAdminCredential() (username, passHash string, ok bool, err error) {
	err = db.sql.QueryRow(
		`SELECT username, pass_hash FROM admin_credentials WHERE id = 1`,
	).Scan(&username, &passHash)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return username, passHash, true, nil
}

// SaveAdminCredential upserts the single operator row. passHash must already
// be an argon2id PHC string (services.HashPassword) — this layer stores it
// verbatim and has no crypto knowledge of its own.
func (db *DB) SaveAdminCredential(username, passHash string) error {
	_, err := db.sql.Exec(
		`INSERT INTO admin_credentials(id, username, pass_hash, updated_at)
		 VALUES (1, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			username = excluded.username,
			pass_hash = excluded.pass_hash,
			updated_at = excluded.updated_at`,
		username, passHash, now(),
	)
	return err
}

// ClearAdminCredential removes the operator row, returning the server to
// setup mode on the next boot. Backs the `shellyctl reset-auth` recovery
// subcommand. Idempotent — clearing an already-empty table is a no-op.
func (db *DB) ClearAdminCredential() error {
	_, err := db.sql.Exec(`DELETE FROM admin_credentials WHERE id = 1`)
	return err
}
