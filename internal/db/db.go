package db

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"shellyadmin/internal/core/secretbox"
	"shellyadmin/internal/models"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type DB struct {
	sql *sql.DB
}

type LogEntry struct {
	ID        int    `json:"id"`
	TS        string `json:"ts"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	dsn := filepath.Join(dataDir, "shellyctl.db")
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		return nil, err
	}
	db := &DB{sql: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	if err := db.encryptPlaintextCredentials(); err != nil {
		return nil, err
	}
	return db, nil
}

// resolveSecret picks between the legacy plaintext column and the cipher_blob
// column depending on what the row actually holds. New rows leave the
// plaintext side empty; pre-upgrade rows still have it populated. If the
// startup sweep has run, every row has cipher_blob populated.
func resolveSecret(plain, cipher string) (string, error) {
	if cipher != "" {
		return secretbox.OpenString(cipher)
	}
	return plain, nil
}

// encryptPlaintextCredentials does a one-shot sweep at Open() time: any row
// with a non-empty plaintext password/ha1 and an empty cipher column gets
// re-written with the encrypted form and the plaintext column cleared.
// After this runs once there is no cleartext secret in the DB file.
//
// Skips silently if the secretbox key has not been installed yet — callers
// that never populate a key (e.g. the odd test that opens the DB without
// provisioning encryption) keep the legacy behaviour.
func (db *DB) encryptPlaintextCredentials() error {
	if !secretbox.HasKey() {
		return nil
	}
	for _, table := range []string{"credentials", "credential_groups"} {
		rows, err := db.sql.Query(fmt.Sprintf(
			`SELECT name, password, ha1, password_cipher, ha1_cipher FROM %s`, table))
		if err != nil {
			return err
		}
		type pending struct {
			name, password, ha1 string
		}
		var todo []pending
		for rows.Next() {
			var name, plainPass, plainHA1, cipherPass, cipherHA1 string
			if err := rows.Scan(&name, &plainPass, &plainHA1, &cipherPass, &cipherHA1); err != nil {
				rows.Close()
				return err
			}
			if plainPass == "" && plainHA1 == "" {
				continue
			}
			// If cipher is already populated we treat it as the source of
			// truth — do not overwrite, just clear plaintext.
			if cipherPass != "" {
				plainPass = ""
			}
			if cipherHA1 != "" {
				plainHA1 = ""
			}
			todo = append(todo, pending{name: name, password: plainPass, ha1: plainHA1})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, p := range todo {
			sealedPass, err := secretbox.SealString(p.password)
			if err != nil {
				return fmt.Errorf("sweep %s %q password: %w", table, p.name, err)
			}
			sealedHA1, err := secretbox.SealString(p.ha1)
			if err != nil {
				return fmt.Errorf("sweep %s %q ha1: %w", table, p.name, err)
			}
			// COALESCE: preserve any cipher column that's already populated.
			if _, err := db.sql.Exec(fmt.Sprintf(
				`UPDATE %s
				SET password='', ha1='',
					password_cipher=CASE WHEN password_cipher='' THEN ? ELSE password_cipher END,
					ha1_cipher=CASE WHEN ha1_cipher='' THEN ? ELSE ha1_cipher END,
					updated_at=?
				WHERE name=?`, table),
				sealedPass, sealedHA1, now(), p.name); err != nil {
				return fmt.Errorf("sweep %s %q update: %w", table, p.name, err)
			}
		}
	}
	return nil
}

func (db *DB) Close() error { return db.sql.Close() }

func (db *DB) MarkRunningJobsInterrupted() error {
	_, err := db.sql.Exec(`UPDATE jobs
		SET status = 'interrupted', error = 'service restarted', updated_at = ?
		WHERE status = 'running'`, now())
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

func (db *DB) ListDevices() ([]models.Device, error) {
	rows, err := db.sql.Query(`SELECT mac, ip, name, model, fw, gen, online, last_seen, first_seen, device_num,
		last_refresh_attempt, last_refresh_ok, last_refresh_error,
		consecutive_misses, mqtt_enabled, mqtt_server, mqtt_client_id, mqtt_topic_prefix, mqtt_flags_na,
		lat, lon, tz, ws_enabled, ws_server, ble_gw_enabled, wifi_ssid,
		fw_available_stable, fw_available_beta, fw_checked_at, fw_auto_update,
		cloud_enabled, cloud_connected, ws_connected, matter_enabled, sntp_server, serial, auth_required, auth_error,
		eco_mode, discoverable, raw_config, raw_status,
		scheme, enhanced_security, tls_cert_valid, tls_allow_insecure, auth_locked_until, wifi_hostname, wifi_channel,
		power_w, voltage_v, current_a
		FROM devices ORDER BY device_num ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Device
	for rows.Next() {
		var d models.Device
		var online, refreshOK, cloudConnected, wsConnected, authRequired int
		if err := rows.Scan(&d.MAC, &d.IP, &d.Name, &d.Model, &d.FW, &d.Gen, &online, &d.LastSeen, &d.FirstSeen, &d.DeviceNum,
			&d.LastRefreshAttempt, &refreshOK, &d.LastRefreshError,
			&d.ConsecutiveMisses, &d.MQTTEnabled, &d.MQTTServer, &d.MQTTClientID, &d.MQTTTopicPrefix, &d.MQTTFlagsNA,
			&d.Lat, &d.Lon, &d.TZ, &d.WSEnabled, &d.WSServer, &d.BLEGWEnabled, &d.WiFiSSID,
			&d.FWAvailableStable, &d.FWAvailableBeta, &d.FWCheckedAt, &d.FWAutoUpdate,
			&d.CloudEnabled, &cloudConnected, &wsConnected, &d.MatterEnabled, &d.SNTPServer, &d.Serial, &authRequired, &d.AuthError,
			&d.EcoMode, &d.Discoverable, &d.RawConfig, &d.RawStatus,
			&d.Scheme, &d.EnhancedSecurity, &d.TLSCertValid, &d.TLSAllowInsecure, &d.AuthLockedUntil, &d.WiFiHostname, &d.WiFiChannel,
			&d.PowerW, &d.VoltageV, &d.CurrentA); err != nil {
			return nil, err
		}
		d.Online = online == 1
		d.LastRefreshOK = refreshOK == 1
		d.CloudConnected = cloudConnected == 1
		d.WSConnected = wsConnected == 1
		d.AuthRequired = authRequired == 1
		out = append(out, d)
	}
	return out, rows.Err()
}

// dbExec is satisfied by both *sql.DB and *sql.Tx, letting the row-level
// upsert helper run inside or outside a transaction.
type dbExec interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func (db *DB) UpsertDevices(scanned []models.Device) error {
	existing, err := db.ListDevices()
	if err != nil {
		return err
	}
	known := map[string]models.Device{}
	maxNum := 0
	for _, d := range existing {
		known[d.MAC] = d
		if d.DeviceNum > maxNum {
			maxNum = d.DeviceNum
		}
	}
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	seen := map[string]bool{}
	for _, d := range scanned {
		seen[d.MAC] = true
		if old, ok := known[d.MAC]; ok {
			d.DeviceNum = old.DeviceNum
			d.FirstSeen = old.FirstSeen
		} else {
			maxNum++
			d.DeviceNum = maxNum
			d.FirstSeen = now()
		}
		d.LastSeen = now()
		d.LastRefreshAttempt = d.LastSeen
		d.LastRefreshOK = true
		d.LastRefreshError = ""
		d.ConsecutiveMisses = 0
		d.Online = true
		if err := upsertDeviceRow(tx, d); err != nil {
			return err
		}
	}
	for _, d := range existing {
		if seen[d.MAC] {
			continue
		}
		d.LastRefreshAttempt = now()
		d.LastRefreshOK = false
		d.LastRefreshError = "refresh timed out"
		d.ConsecutiveMisses++
		if d.ConsecutiveMisses >= 2 {
			d.Online = false
		}
		if err := upsertDeviceRow(tx, d); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func upsertDeviceRow(ex dbExec, d models.Device) error {
	if d.Scheme == "" {
		d.Scheme = "http"
	}
	_, err := ex.Exec(`INSERT INTO devices (
		mac, ip, name, model, fw, gen, online, last_seen, first_seen, device_num, consecutive_misses,
		last_refresh_attempt, last_refresh_ok, last_refresh_error,
		mqtt_enabled, mqtt_server, mqtt_client_id, mqtt_topic_prefix, mqtt_flags_na, lat, lon, tz,
		ws_enabled, ws_server, ble_gw_enabled, wifi_ssid,
		fw_available_stable, fw_available_beta, fw_checked_at, fw_auto_update,
		cloud_enabled, auth_required, auth_error,
		cloud_connected, ws_connected, matter_enabled, sntp_server, serial, eco_mode, discoverable, raw_config, raw_status,
		scheme, enhanced_security, tls_cert_valid, tls_allow_insecure, auth_locked_until, wifi_hostname, wifi_channel,
		power_w, voltage_v, current_a
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(mac) DO UPDATE SET
		ip=excluded.ip, name=excluded.name, model=excluded.model, fw=excluded.fw, gen=excluded.gen,
		online=excluded.online, last_seen=excluded.last_seen, first_seen=excluded.first_seen,
		device_num=excluded.device_num, consecutive_misses=excluded.consecutive_misses,
		last_refresh_attempt=excluded.last_refresh_attempt, last_refresh_ok=excluded.last_refresh_ok, last_refresh_error=excluded.last_refresh_error,
		mqtt_enabled=excluded.mqtt_enabled, mqtt_server=excluded.mqtt_server,
		mqtt_client_id=excluded.mqtt_client_id, mqtt_topic_prefix=excluded.mqtt_topic_prefix,
		mqtt_flags_na=excluded.mqtt_flags_na, lat=excluded.lat, lon=excluded.lon, tz=excluded.tz,
		ws_enabled=excluded.ws_enabled, ws_server=excluded.ws_server, ble_gw_enabled=excluded.ble_gw_enabled,
		wifi_ssid=excluded.wifi_ssid,
		fw_available_stable=excluded.fw_available_stable,
		fw_available_beta=excluded.fw_available_beta,
		fw_checked_at=excluded.fw_checked_at,
		fw_auto_update=excluded.fw_auto_update,
		auth_required=excluded.auth_required, auth_error=excluded.auth_error,
		cloud_enabled=excluded.cloud_enabled, cloud_connected=excluded.cloud_connected,
		ws_connected=excluded.ws_connected, matter_enabled=excluded.matter_enabled,
		sntp_server=excluded.sntp_server, serial=excluded.serial,
		eco_mode=excluded.eco_mode, discoverable=excluded.discoverable,
		raw_config=excluded.raw_config, raw_status=excluded.raw_status,
		scheme=excluded.scheme, enhanced_security=excluded.enhanced_security,
		tls_cert_valid=excluded.tls_cert_valid, tls_allow_insecure=excluded.tls_allow_insecure,
		auth_locked_until=excluded.auth_locked_until,
		wifi_hostname=excluded.wifi_hostname, wifi_channel=excluded.wifi_channel,
		power_w=excluded.power_w, voltage_v=excluded.voltage_v, current_a=excluded.current_a`,
		d.MAC, d.IP, d.Name, d.Model, d.FW, d.Gen, boolToInt(d.Online), d.LastSeen, d.FirstSeen, d.DeviceNum, d.ConsecutiveMisses,
		d.LastRefreshAttempt, boolToInt(d.LastRefreshOK), d.LastRefreshError,
		d.MQTTEnabled, d.MQTTServer, d.MQTTClientID, d.MQTTTopicPrefix, d.MQTTFlagsNA, d.Lat, d.Lon, d.TZ,
		d.WSEnabled, d.WSServer, d.BLEGWEnabled, d.WiFiSSID,
		d.FWAvailableStable, d.FWAvailableBeta, d.FWCheckedAt, d.FWAutoUpdate,
		d.CloudEnabled, boolToInt(d.AuthRequired), d.AuthError,
		boolToInt(d.CloudConnected), boolToInt(d.WSConnected), d.MatterEnabled, d.SNTPServer, d.Serial, d.EcoMode, d.Discoverable, d.RawConfig, d.RawStatus,
		d.Scheme, d.EnhancedSecurity, d.TLSCertValid, d.TLSAllowInsecure, d.AuthLockedUntil, d.WiFiHostname, d.WiFiChannel,
		d.PowerW, d.VoltageV, d.CurrentA)
	return err
}

func (db *DB) ForgetDevice(target string) error {
	_, err := db.sql.Exec(`DELETE FROM devices WHERE mac = ? OR ip = ?`, target, target)
	return err
}

func (db *DB) UpsertDevice(device models.Device) error {
	return upsertDeviceRow(db.sql, device)
}

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

func (db *DB) CreateJob(jobType, restartPolicy, payload string, total int) (int64, error) {
	res, err := db.sql.Exec(`INSERT INTO jobs(type, status, restart_policy, done, total, payload, result, error, created_at, updated_at)
		VALUES (?, 'running', ?, 0, ?, ?, '', '', ?, ?)`, jobType, restartPolicy, total, payload, now(), now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) UpdateJobProgress(id int64, done, total int, result string) error {
	_, err := db.sql.Exec(`UPDATE jobs
		SET done = ?, total = ?, result = ?, updated_at = ?
		WHERE id = ?`, done, total, result, now(), id)
	return err
}

func (db *DB) IncrementJobDone(id int64) error {
	_, err := db.sql.Exec(`UPDATE jobs
		SET done = done + 1, updated_at = ?
		WHERE id = ? AND status = 'running'`, now(), id)
	return err
}

func (db *DB) CompleteJob(id int64, status, result, errText string, done, total int) error {
	_, err := db.sql.Exec(`UPDATE jobs
		SET status = ?, result = ?, error = ?, done = ?, total = ?, updated_at = ?
		WHERE id = ?`, status, result, errText, done, total, now(), id)
	return err
}

func (db *DB) InterruptJob(id int64, errText string) error {
	_, err := db.sql.Exec(`UPDATE jobs
		SET status = 'interrupted', error = ?, updated_at = ?
		WHERE id = ? AND status = 'running'`, errText, now(), id)
	return err
}

func (db *DB) GetLatestJob(jobType string) (models.Job, error) {
	row := db.sql.QueryRow(`SELECT id, type, status, restart_policy, done, total, payload, result, error, created_at, updated_at
		FROM jobs WHERE type = ? ORDER BY id DESC LIMIT 1`, jobType)
	return scanJob(row)
}

func (db *DB) GetJob(id int64) (models.Job, error) {
	row := db.sql.QueryRow(`SELECT id, type, status, restart_policy, done, total, payload, result, error, created_at, updated_at
		FROM jobs WHERE id = ?`, id)
	return scanJob(row)
}

func (db *DB) ListInterruptedRestartableJobs() ([]models.Job, error) {
	rows, err := db.sql.Query(`SELECT id, type, status, restart_policy, done, total, payload, result, error, created_at, updated_at
		FROM jobs
		WHERE status = 'interrupted' AND restart_policy = 'auto'
		ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func (db *DB) ListTemplateNames() ([]string, error) {
	rows, err := db.sql.Query(`SELECT name FROM templates ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func (db *DB) ListTemplates() (map[string]string, error) {
	rows, err := db.sql.Query(`SELECT name, content FROM templates ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var name, content string
		if err := rows.Scan(&name, &content); err != nil {
			return nil, err
		}
		out[name] = content
	}
	return out, rows.Err()
}

func (db *DB) GetTemplate(name string) (string, string, error) {
	var content, credentialRef string
	err := db.sql.QueryRow(`SELECT content, credential_ref FROM templates WHERE name = ?`, name).Scan(&content, &credentialRef)
	return content, credentialRef, err
}

func (db *DB) SaveTemplate(name, content, credentialRef string) error {
	_, err := db.sql.Exec(`INSERT INTO templates(name, content, credential_ref, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET content=excluded.content, credential_ref=excluded.credential_ref`, name, content, credentialRef, now())
	return err
}

func (db *DB) DeleteTemplate(name string) error {
	_, err := db.sql.Exec(`DELETE FROM templates WHERE name = ?`, name)
	return err
}

func (db *DB) ListCredentials() ([]models.Credential, error) {
	rows, err := db.sql.Query(`SELECT name, username, password, ha1, password_cipher, ha1_cipher, tags FROM credentials ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Credential{}
	for rows.Next() {
		var c models.Credential
		var plainPassword, plainHA1, passwordCipher, ha1Cipher, tagsRaw string
		if err := rows.Scan(&c.Name, &c.Username, &plainPassword, &plainHA1, &passwordCipher, &ha1Cipher, &tagsRaw); err != nil {
			return nil, err
		}
		c.Password, err = resolveSecret(plainPassword, passwordCipher)
		if err != nil {
			return nil, fmt.Errorf("credential %q password decrypt: %w", c.Name, err)
		}
		c.HA1, err = resolveSecret(plainHA1, ha1Cipher)
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
	var plainPassword, plainHA1, passwordCipher, ha1Cipher, tagsRaw string
	err := db.sql.QueryRow(`SELECT name, username, password, ha1, password_cipher, ha1_cipher, tags FROM credentials WHERE name = ?`, name).Scan(&c.Name, &c.Username, &plainPassword, &plainHA1, &passwordCipher, &ha1Cipher, &tagsRaw)
	if err != nil {
		return models.Credential{}, err
	}
	c.Password, err = resolveSecret(plainPassword, passwordCipher)
	if err != nil {
		return models.Credential{}, fmt.Errorf("credential %q password decrypt: %w", c.Name, err)
	}
	c.HA1, err = resolveSecret(plainHA1, ha1Cipher)
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
	// New rows only populate the cipher columns; the plaintext columns stay
	// empty so anything reading the SQLite file sees no sensitive material.
	_, err = db.sql.Exec(`INSERT INTO credentials(name, username, password, ha1, password_cipher, ha1_cipher, tags, created_at, updated_at)
		VALUES (?, ?, '', '', ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			username=excluded.username,
			password='',
			ha1='',
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
	rows, err := db.sql.Query(`SELECT name, password, ha1, password_cipher, ha1_cipher, tags FROM credential_groups ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CredentialGroup{}
	for rows.Next() {
		var g models.CredentialGroup
		var plainPassword, plainHA1, passwordCipher, ha1Cipher, tagsRaw string
		if err := rows.Scan(&g.Name, &plainPassword, &plainHA1, &passwordCipher, &ha1Cipher, &tagsRaw); err != nil {
			return nil, err
		}
		g.Password, err = resolveSecret(plainPassword, passwordCipher)
		if err != nil {
			return nil, fmt.Errorf("group %q password decrypt: %w", g.Name, err)
		}
		g.HA1, err = resolveSecret(plainHA1, ha1Cipher)
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
	_, err = db.sql.Exec(`INSERT INTO credential_groups(name, credential_ref, password, ha1, password_cipher, ha1_cipher, tags, created_at, updated_at)
		VALUES (?, ?, '', '', ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			credential_ref=excluded.credential_ref,
			password='',
			ha1='',
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

func (db *DB) AddLog(level, message string) error {
	return db.AddLogWithRequestID(level, message, "")
}

// AddLogWithRequestID persists an audit entry tagged with the originating
// HTTP request's correlation ID (empty for jobs triggered outside a request).
func (db *DB) AddLogWithRequestID(level, message, requestID string) error {
	_, err := db.sql.Exec(
		`INSERT INTO audit_log(ts, level, message, request_id) VALUES (?, ?, ?, ?)`,
		now(), level, message, requestID,
	)
	return err
}

func (db *DB) GetLogs(level, search string) ([]LogEntry, error) {
	query := `SELECT id, ts, level, message, request_id FROM audit_log WHERE 1=1`
	args := []any{}
	if level != "" {
		query += ` AND level = ?`
		args = append(args, strings.ToUpper(level))
	}
	if search != "" {
		query += ` AND message LIKE ?`
		args = append(args, "%"+search+"%")
	}
	query += ` ORDER BY id DESC LIMIT 500`
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		var entry LogEntry
		if err := rows.Scan(&entry.ID, &entry.TS, &entry.Level, &entry.Message, &entry.RequestID); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (db *DB) GetLogsForExport(level, search string, limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 100000
	}
	query := `SELECT id, ts, level, message, request_id FROM audit_log WHERE 1=1`
	args := []any{}
	if level != "" {
		query += ` AND level = ?`
		args = append(args, strings.ToUpper(level))
	}
	if search != "" {
		query += ` AND message LIKE ?`
		args = append(args, "%"+search+"%")
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		var entry LogEntry
		if err := rows.Scan(&entry.ID, &entry.TS, &entry.Level, &entry.Message, &entry.RequestID); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (db *DB) ClearLogs() (int64, error) {
	res, err := db.sql.Exec(`DELETE FROM audit_log`)
	if err != nil {
		return 0, err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return count, nil
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

func scanJob(scanner interface{ Scan(dest ...any) error }) (models.Job, error) {
	var job models.Job
	err := scanner.Scan(&job.ID, &job.Type, &job.Status, &job.RestartPolicy, &job.Done, &job.Total, &job.Payload, &job.Result, &job.Error, &job.CreatedAt, &job.UpdatedAt)
	return job, err
}
