package db

// Device-credential + credential-group persistence (secretbox-sealed at
// rest) and the device→group assignment table. MOVED FROM db.go — db-layer
// split by domain (post-v0.5.2 review item 6); bodies unchanged. The
// operator-login credential lives in admin_credentials.go.

import (
	"encoding/json"
	"fmt"
	"strings"

	"shellyadmin/internal/core/secretbox"
	"shellyadmin/internal/models"
)

func (db *DB) ListCredentials() ([]models.Credential, error) {
	rows, err := db.sql.Query(`SELECT name, username, password_cipher, ha1_cipher, tags FROM credentials ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Credential{}
	for rows.Next() {
		var c models.Credential
		var passwordCipher, ha1Cipher, tagsRaw string
		if err := rows.Scan(&c.Name, &c.Username, &passwordCipher, &ha1Cipher, &tagsRaw); err != nil {
			return nil, err
		}
		c.Password, err = decryptCipher(passwordCipher)
		if err != nil {
			return nil, fmt.Errorf("credential %q password decrypt: %w", c.Name, err)
		}
		c.HA1, err = decryptCipher(ha1Cipher)
		if err != nil {
			return nil, fmt.Errorf("credential %q ha1 decrypt: %w", c.Name, err)
		}
		_ = json.Unmarshal([]byte(tagsRaw), &c.Tags)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (db *DB) GetCredential(name string) (models.Credential, error) {
	var c models.Credential
	var passwordCipher, ha1Cipher, tagsRaw string
	err := db.sql.QueryRow(`SELECT name, username, password_cipher, ha1_cipher, tags FROM credentials WHERE name = ?`, name).Scan(&c.Name, &c.Username, &passwordCipher, &ha1Cipher, &tagsRaw)
	if err != nil {
		return models.Credential{}, err
	}
	c.Password, err = decryptCipher(passwordCipher)
	if err != nil {
		return models.Credential{}, fmt.Errorf("credential %q password decrypt: %w", c.Name, err)
	}
	c.HA1, err = decryptCipher(ha1Cipher)
	if err != nil {
		return models.Credential{}, fmt.Errorf("credential %q ha1 decrypt: %w", c.Name, err)
	}
	_ = json.Unmarshal([]byte(tagsRaw), &c.Tags)
	return c, nil
}

func (db *DB) SaveCredential(c models.Credential) error {
	tagsBody, err := json.Marshal(c.Tags)
	if err != nil {
		return err
	}
	passwordCipher, err := secretbox.SealString(c.Password)
	if err != nil {
		return fmt.Errorf("credential %q password encrypt: %w", c.Name, err)
	}
	ha1Cipher, err := secretbox.SealString(c.HA1)
	if err != nil {
		return fmt.Errorf("credential %q ha1 encrypt: %w", c.Name, err)
	}
	_, err = db.sql.Exec(`INSERT INTO credentials(name, username, password_cipher, ha1_cipher, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			username=excluded.username,
			password_cipher=excluded.password_cipher,
			ha1_cipher=excluded.ha1_cipher,
			tags=excluded.tags,
			updated_at=excluded.updated_at`,
		c.Name, c.Username, passwordCipher, ha1Cipher, string(tagsBody), now(), now())
	return err
}

func (db *DB) DeleteCredential(name string) error {
	_, err := db.sql.Exec(`DELETE FROM credentials WHERE name = ?`, name)
	return err
}

func (db *DB) ListCredentialGroups() ([]models.CredentialGroup, error) {
	rows, err := db.sql.Query(`SELECT name, password_cipher, ha1_cipher, tags FROM credential_groups ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CredentialGroup{}
	for rows.Next() {
		var g models.CredentialGroup
		var passwordCipher, ha1Cipher, tagsRaw string
		if err := rows.Scan(&g.Name, &passwordCipher, &ha1Cipher, &tagsRaw); err != nil {
			return nil, err
		}
		g.Password, err = decryptCipher(passwordCipher)
		if err != nil {
			return nil, fmt.Errorf("group %q password decrypt: %w", g.Name, err)
		}
		g.HA1, err = decryptCipher(ha1Cipher)
		if err != nil {
			return nil, fmt.Errorf("group %q ha1 decrypt: %w", g.Name, err)
		}
		_ = json.Unmarshal([]byte(tagsRaw), &g.Tags)
		out = append(out, g)
	}
	return out, rows.Err()
}

func (db *DB) SaveCredentialGroup(group models.CredentialGroup) error {
	tagsBody, err := json.Marshal(group.Tags)
	if err != nil {
		return err
	}
	passwordCipher, err := secretbox.SealString(group.Password)
	if err != nil {
		return fmt.Errorf("group %q password encrypt: %w", group.Name, err)
	}
	ha1Cipher, err := secretbox.SealString(group.HA1)
	if err != nil {
		return fmt.Errorf("group %q ha1 encrypt: %w", group.Name, err)
	}
	_, err = db.sql.Exec(`INSERT INTO credential_groups(name, credential_ref, password_cipher, ha1_cipher, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			credential_ref=excluded.credential_ref,
			password_cipher=excluded.password_cipher,
			ha1_cipher=excluded.ha1_cipher,
			tags=excluded.tags,
			updated_at=excluded.updated_at`,
		group.Name, group.Name, passwordCipher, ha1Cipher, string(tagsBody), now(), now())
	return err
}

func (db *DB) DeleteCredentialGroup(name string) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM device_credential_groups WHERE group_name = ?`, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM credential_groups WHERE name = ?`, name); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) ListDeviceCredentialGroupAssignments() ([]models.DeviceCredentialGroupAssignment, error) {
	rows, err := db.sql.Query(`SELECT mac, group_name FROM device_credential_groups ORDER BY mac ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.DeviceCredentialGroupAssignment{}
	for rows.Next() {
		var a models.DeviceCredentialGroupAssignment
		if err := rows.Scan(&a.MAC, &a.GroupName); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (db *DB) SaveDeviceCredentialGroupAssignments(macs []string, groupName string) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, mac := range macs {
		if strings.TrimSpace(groupName) == "" {
			if _, err := tx.Exec(`DELETE FROM device_credential_groups WHERE mac = ?`, mac); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(`INSERT INTO device_credential_groups(mac, group_name, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(mac) DO UPDATE SET
				group_name=excluded.group_name,
				updated_at=excluded.updated_at`,
			mac, groupName, now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) ReplaceDeviceCredentialGroupAssignments(assignments map[string]string) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM device_credential_groups`); err != nil {
		return err
	}
	for mac, groupName := range assignments {
		if strings.TrimSpace(mac) == "" || strings.TrimSpace(groupName) == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO device_credential_groups(mac, group_name, updated_at) VALUES (?, ?, ?)`, mac, groupName, now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
