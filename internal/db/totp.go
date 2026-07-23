package db

// TOTP-2FA enrollment rows (T1). MOVED FROM db.go — db-layer split by
// domain (post-v0.5.2 review item 6); bodies unchanged.

// TOTPState is the per-user TOTP-2FA envelope (T1 from the consolidated
// review). SecretCipher and BackupCodesCipher hold secretbox-sealed
// payloads — the bare TOTP secret never leaves the process. A user
// with no row is treated as "not enrolled"; the login path skips the
// second-factor step until SetTOTP commits a row.
type TOTPState struct {
	Username          string
	SecretCipher      string
	EnrolledAt        string
	LastVerifiedAt    string
	BackupCodesCipher string
	BackupCodesUsed   int
}

// GetTOTP returns the row for username, or sql.ErrNoRows when the user
// hasn't enrolled. Callers treat ErrNoRows as "TOTP not required for
// this user" — enrolled operators are required to enter a code,
// non-enrolled operators can still log in password-only.
func (db *DB) GetTOTP(username string) (TOTPState, error) {
	var t TOTPState
	err := db.sql.QueryRow(
		`SELECT username, secret_cipher, enrolled_at, last_verified_at, backup_codes_cipher, backup_codes_used
		 FROM totp_state WHERE username = ?`,
		username,
	).Scan(&t.Username, &t.SecretCipher, &t.EnrolledAt, &t.LastVerifiedAt, &t.BackupCodesCipher, &t.BackupCodesUsed)
	return t, err
}

// SetTOTP upserts the per-user row. Used on enrollment commit and on
// every successful login (to bump last_verified_at + backup_codes_used).
func (db *DB) SetTOTP(state TOTPState) error {
	_, err := db.sql.Exec(
		`INSERT INTO totp_state(username, secret_cipher, enrolled_at, last_verified_at, backup_codes_cipher, backup_codes_used)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(username) DO UPDATE SET
			secret_cipher = excluded.secret_cipher,
			enrolled_at = excluded.enrolled_at,
			last_verified_at = excluded.last_verified_at,
			backup_codes_cipher = excluded.backup_codes_cipher,
			backup_codes_used = excluded.backup_codes_used`,
		state.Username, state.SecretCipher, state.EnrolledAt,
		state.LastVerifiedAt, state.BackupCodesCipher, state.BackupCodesUsed,
	)
	return err
}

// DeleteTOTP removes the user's row. Used by the operator-initiated
// "disable 2FA" flow (after a fresh TOTP/backup verify) and by the
// `shellyctl totp-reset <user>` recovery subcommand.
func (db *DB) DeleteTOTP(username string) error {
	_, err := db.sql.Exec(`DELETE FROM totp_state WHERE username = ?`, username)
	return err
}
