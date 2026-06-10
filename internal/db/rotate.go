package db

// Encryption-key rotation: re-seal every secretbox-sealed column under a
// new key in one transaction. Backs `shellyctl rotate-key` (review item 8 —
// replaces the manual clear-everything playbook in docs/SECURITY.md).

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"shellyadmin/internal/core/secretbox"
)

// RotationReport counts what RotateSealedColumns touched, for operator
// feedback. A surface with no sealed rows reports zero and is not an error.
type RotationReport struct {
	Credentials      int
	CredentialGroups int
	TOTPUsers        int
	MCPToken         bool
}

// RotateSealedColumns walks every secretbox-sealed column — credentials and
// credential_groups (password_cipher, ha1_cipher), totp_state (secret_cipher,
// backup_codes_cipher), and the mcp_token inside the settings JSON row — and
// re-encrypts each blob via open (old key) + seal (new key). Everything runs
// in ONE transaction: a failure on any row (typically: a blob the old key
// cannot open) rolls back the whole rotation, so the database is never left
// half-rotated.
//
// apply=false is the dry-run mode: the same reads and open-checks run (so a
// wrong old key is caught), but nothing is written and the transaction is
// rolled back. Columns that are empty or not in blob format (legacy
// plaintext) are left untouched in both modes. The admin login is argon2id,
// not sealed, and is deliberately not part of this.
func (db *DB) RotateSealedColumns(open, seal func(string) (string, error), apply bool) (RotationReport, error) {
	report := RotationReport{}
	tx, err := db.sql.Begin()
	if err != nil {
		return report, err
	}
	defer func() { _ = tx.Rollback() }()

	reseal := func(cipher string) (string, bool, error) {
		if cipher == "" || !secretbox.IsBlob(cipher) {
			return cipher, false, nil
		}
		plain, err := open(cipher)
		if err != nil {
			return "", false, err
		}
		out, err := seal(plain)
		if err != nil {
			return "", false, err
		}
		return out, true, nil
	}

	// credentials + credential_groups share the same two-cipher shape.
	for _, table := range []string{"credentials", "credential_groups"} {
		rows, err := tx.Query(`SELECT name, password_cipher, ha1_cipher FROM ` + table)
		if err != nil {
			return report, err
		}
		type pending struct{ name, password, ha1 string }
		var updates []pending
		for rows.Next() {
			var name, passwordCipher, ha1Cipher string
			if err := rows.Scan(&name, &passwordCipher, &ha1Cipher); err != nil {
				rows.Close()
				return report, err
			}
			newPassword, touchedPassword, err := reseal(passwordCipher)
			if err != nil {
				rows.Close()
				return report, fmt.Errorf("%s %q password: %w", table, name, err)
			}
			newHA1, touchedHA1, err := reseal(ha1Cipher)
			if err != nil {
				rows.Close()
				return report, fmt.Errorf("%s %q ha1: %w", table, name, err)
			}
			if touchedPassword || touchedHA1 {
				updates = append(updates, pending{name: name, password: newPassword, ha1: newHA1})
			}
		}
		if err := rows.Close(); err != nil {
			return report, err
		}
		for _, u := range updates {
			if apply {
				if _, err := tx.Exec(`UPDATE `+table+` SET password_cipher = ?, ha1_cipher = ? WHERE name = ?`, u.password, u.ha1, u.name); err != nil {
					return report, err
				}
			}
			if table == "credentials" {
				report.Credentials++
			} else {
				report.CredentialGroups++
			}
		}
	}

	// totp_state: secret + backup codes per user.
	{
		rows, err := tx.Query(`SELECT username, secret_cipher, backup_codes_cipher FROM totp_state`)
		if err != nil {
			return report, err
		}
		type pending struct{ username, secret, backup string }
		var updates []pending
		for rows.Next() {
			var username, secretCipher, backupCipher string
			if err := rows.Scan(&username, &secretCipher, &backupCipher); err != nil {
				rows.Close()
				return report, err
			}
			newSecret, touchedSecret, err := reseal(secretCipher)
			if err != nil {
				rows.Close()
				return report, fmt.Errorf("totp %q secret: %w", username, err)
			}
			newBackup, touchedBackup, err := reseal(backupCipher)
			if err != nil {
				rows.Close()
				return report, fmt.Errorf("totp %q backup codes: %w", username, err)
			}
			if touchedSecret || touchedBackup {
				updates = append(updates, pending{username: username, secret: newSecret, backup: newBackup})
			}
		}
		if err := rows.Close(); err != nil {
			return report, err
		}
		for _, u := range updates {
			if apply {
				if _, err := tx.Exec(`UPDATE totp_state SET secret_cipher = ?, backup_codes_cipher = ? WHERE username = ?`, u.secret, u.backup, u.username); err != nil {
					return report, err
				}
			}
			report.TOTPUsers++
		}
	}

	// settings key='app': the MCP token is a sealed blob inside the JSON
	// value. Round-trip through a generic map (not models.AppSettings) so
	// rotation cannot drop or normalize any other field.
	{
		var raw string
		err := tx.QueryRow(`SELECT value FROM settings WHERE key='app'`).Scan(&raw)
		switch {
		case err == nil:
			var settings map[string]any
			if err := json.Unmarshal([]byte(raw), &settings); err != nil {
				return report, fmt.Errorf("settings row: %w", err)
			}
			if token, ok := settings["mcp_token"].(string); ok && secretbox.IsBlob(token) {
				newToken, touched, err := reseal(token)
				if err != nil {
					return report, fmt.Errorf("settings mcp_token: %w", err)
				}
				if touched {
					settings["mcp_token"] = newToken
					body, err := json.Marshal(settings)
					if err != nil {
						return report, err
					}
					if apply {
						if _, err := tx.Exec(`UPDATE settings SET value = ? WHERE key='app'`, string(body)); err != nil {
							return report, err
						}
					}
					report.MCPToken = true
				}
			}
		case errors.Is(err, sql.ErrNoRows):
			// No settings row yet — nothing to rotate.
		default:
			return report, err
		}
	}

	if !apply {
		return report, nil
	}
	return report, tx.Commit()
}
