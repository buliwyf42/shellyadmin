package db

// AppSettings persistence (single JSON row under settings.key='app').
// MOVED FROM db.go — db-layer split by domain (post-v0.5.2 review item 6);
// bodies unchanged.

import (
	"database/sql"
	"encoding/json"

	"shellyadmin/internal/models"
)

func (db *DB) GetSettings() (models.AppSettings, error) {
	var raw string
	err := db.sql.QueryRow(`SELECT value FROM settings WHERE key='app'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return models.DefaultSettings(), nil
	}
	if err != nil {
		return models.AppSettings{}, err
	}
	var settings models.AppSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return models.AppSettings{}, err
	}
	if settings.ScanConcurrency == 0 {
		settings.ScanConcurrency = 64
	}
	if settings.ScanTimeout == 0 {
		settings.ScanTimeout = 2
	}
	if settings.RefreshTimeout == 0 {
		settings.RefreshTimeout = 5
	}
	settings.Normalize()
	return settings, nil
}

func (db *DB) SaveSettings(settings models.AppSettings) error {
	settings.Normalize()
	body, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = db.sql.Exec(`INSERT INTO settings(key, value) VALUES ('app', ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(body))
	return err
}
