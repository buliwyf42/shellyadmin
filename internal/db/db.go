package db

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
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
	// RiskLevel is set on audit rows that record an action execution
	// (catalog risk: low/medium/high). Empty on every other audit row,
	// including HTTP request logs and job lifecycle events.
	RiskLevel string `json:"risk_level,omitempty"`
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

// marshalSupportedMethods serialises a method-list slice for storage in the
// devices.supported_methods TEXT column. Returns "" for nil/empty slices so
// the on-disk representation distinguishes "never probed" from "probed and
// found to support nothing" (the latter shouldn't happen in practice but
// would round-trip as "[]" rather than "").
func marshalSupportedMethods(methods []string) string {
	if methods == nil {
		return ""
	}
	body, err := json.Marshal(methods)
	if err != nil {
		return ""
	}
	return string(body)
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
	rows, err := db.sql.Query(`SELECT mac, ip, name, model, app, batch, fw_id, fw, gen, online, last_seen, first_seen, device_num,
		last_refresh_attempt, last_refresh_ok, last_refresh_error,
		consecutive_misses, mqtt_enabled, mqtt_server, mqtt_client_id, mqtt_topic_prefix, mqtt_flags_na,
		lat, lon, tz, ws_enabled, ws_server, ble_gw_enabled, wifi_ssid,
		fw_available_stable, fw_available_beta, fw_checked_at, fw_auto_update, supported_methods,
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
		var supportedMethodsRaw string
		if err := rows.Scan(&d.MAC, &d.IP, &d.Name, &d.Model, &d.App, &d.Batch, &d.FWID, &d.FW, &d.Gen, &online, &d.LastSeen, &d.FirstSeen, &d.DeviceNum,
			&d.LastRefreshAttempt, &refreshOK, &d.LastRefreshError,
			&d.ConsecutiveMisses, &d.MQTTEnabled, &d.MQTTServer, &d.MQTTClientID, &d.MQTTTopicPrefix, &d.MQTTFlagsNA,
			&d.Lat, &d.Lon, &d.TZ, &d.WSEnabled, &d.WSServer, &d.BLEGWEnabled, &d.WiFiSSID,
			&d.FWAvailableStable, &d.FWAvailableBeta, &d.FWCheckedAt, &d.FWAutoUpdate, &supportedMethodsRaw,
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
		if supportedMethodsRaw != "" {
			_ = json.Unmarshal([]byte(supportedMethodsRaw), &d.SupportedMethods)
		}
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
		mac, ip, name, model, app, batch, fw_id, fw, gen, online, last_seen, first_seen, device_num, consecutive_misses,
		last_refresh_attempt, last_refresh_ok, last_refresh_error,
		mqtt_enabled, mqtt_server, mqtt_client_id, mqtt_topic_prefix, mqtt_flags_na, lat, lon, tz,
		ws_enabled, ws_server, ble_gw_enabled, wifi_ssid,
		fw_available_stable, fw_available_beta, fw_checked_at, fw_auto_update, supported_methods,
		cloud_enabled, auth_required, auth_error,
		cloud_connected, ws_connected, matter_enabled, sntp_server, serial, eco_mode, discoverable, raw_config, raw_status,
		scheme, enhanced_security, tls_cert_valid, tls_allow_insecure, auth_locked_until, wifi_hostname, wifi_channel,
		power_w, voltage_v, current_a
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(mac) DO UPDATE SET
		ip=excluded.ip, name=excluded.name, model=excluded.model, app=excluded.app, batch=excluded.batch, fw_id=excluded.fw_id, fw=excluded.fw, gen=excluded.gen,
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
		supported_methods=excluded.supported_methods,
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
		d.MAC, d.IP, d.Name, d.Model, d.App, d.Batch, d.FWID, d.FW, d.Gen, boolToInt(d.Online), d.LastSeen, d.FirstSeen, d.DeviceNum, d.ConsecutiveMisses,
		d.LastRefreshAttempt, boolToInt(d.LastRefreshOK), d.LastRefreshError,
		d.MQTTEnabled, d.MQTTServer, d.MQTTClientID, d.MQTTTopicPrefix, d.MQTTFlagsNA, d.Lat, d.Lon, d.TZ,
		d.WSEnabled, d.WSServer, d.BLEGWEnabled, d.WiFiSSID,
		d.FWAvailableStable, d.FWAvailableBeta, d.FWCheckedAt, d.FWAutoUpdate, marshalSupportedMethods(d.SupportedMethods),
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

func (db *DB) AddLog(level, message string) error {
	return db.AddLogWithRequestID(level, message, "")
}

// AddLogWithRequestID persists an audit entry tagged with the originating
// HTTP request's correlation ID (empty for jobs triggered outside a request).
func (db *DB) AddLogWithRequestID(level, message, requestID string) error {
	return db.AddLogWithAttrs(level, message, requestID, "")
}

// AddLogWithAttrs is the full audit-write surface, accepting structured
// attributes the higher layers want preserved alongside the message body.
// `riskLevel` is empty for non-action rows; action-execution rows pass the
// catalog risk so a future compliance query can SELECT WHERE risk_level
// IN (...) without regex-parsing the message body.
//
// S2 — also writes a `prev_hash` chain link: SHA-256 hex of the previous
// row's serialised "ts|level|message|request_id|risk_level|prev_hash"
// form. Verifying the chain (services.VerifyAuditChain) walks rows in
// id order, recomputes the link, and reports any mismatch. A tamperer
// who deletes a row after-the-fact breaks the chain at the next link.
func (db *DB) AddLogWithAttrs(level, message, requestID, riskLevel string) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Look up the most recent row's hash-chain anchor; empty when the
	// table is empty (chain bootstrap).
	var prevTS, prevLevel, prevMsg, prevReqID, prevRisk, prevHash string
	err = tx.QueryRow(
		`SELECT ts, level, message, request_id, risk_level, prev_hash
		 FROM audit_log ORDER BY id DESC LIMIT 1`,
	).Scan(&prevTS, &prevLevel, &prevMsg, &prevReqID, &prevRisk, &prevHash)
	chainAnchor := ""
	if err == nil {
		chainAnchor = chainLink(prevTS, prevLevel, prevMsg, prevReqID, prevRisk, prevHash)
	} else if err != sql.ErrNoRows {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO audit_log(ts, level, message, request_id, risk_level, prev_hash) VALUES (?, ?, ?, ?, ?, ?)`,
		now(), level, message, requestID, riskLevel, chainAnchor,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// chainLink is the canonical serialisation of an audit row that feeds
// the SHA-256 chain hash. Pipe-separated fields; the field names are
// fixed by this definition — adding a column to audit_log without
// extending chainLink invalidates the chain.
func chainLink(ts, level, message, requestID, riskLevel, prevHash string) string {
	body := ts + "|" + level + "|" + message + "|" + requestID + "|" + riskLevel + "|" + prevHash
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// VerifyAuditChain walks the audit_log table in id order and recomputes
// the chain. Returns the id of the first mismatching row, or 0 if the
// chain is intact end-to-end. Used by the operator-facing
// `shellyctl audit-verify` subcommand and by retention-test fixtures.
func (db *DB) VerifyAuditChain() (int64, error) {
	rows, err := db.sql.Query(
		`SELECT id, ts, level, message, request_id, risk_level, prev_hash
		 FROM audit_log ORDER BY id ASC`,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	expectedPrev := ""
	for rows.Next() {
		var id int64
		var ts, level, msg, reqID, risk, prevHash string
		if err := rows.Scan(&id, &ts, &level, &msg, &reqID, &risk, &prevHash); err != nil {
			return 0, err
		}
		if prevHash != expectedPrev {
			return id, nil
		}
		expectedPrev = chainLink(ts, level, msg, reqID, risk, prevHash)
	}
	return 0, rows.Err()
}

// PruneAuditLogOlderThan deletes rows whose ts is strictly older than
// the cutoff. Uses a controlled bypass of the audit_log_no_delete
// trigger (via the __retention_bypass settings flag flipped inside a
// transaction). Returns the number of rows removed. S1 from the
// consolidated review — keeps the table from growing unboundedly on
// long-running operator deployments.
func (db *DB) PruneAuditLogOlderThan(cutoff time.Time) (int64, error) {
	tx, err := db.sql.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	// Flip the bypass flag — the trigger reads it from `settings`.
	if _, err := tx.Exec(
		`INSERT INTO settings(key, value) VALUES ('__retention_bypass', '1')
		 ON CONFLICT(key) DO UPDATE SET value='1'`,
	); err != nil {
		return 0, err
	}
	res, err := tx.Exec(`DELETE FROM audit_log WHERE ts < ?`, cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	// Clear the bypass flag inside the same transaction so a crash
	// between flip and clear leaves the protection intact.
	if _, err := tx.Exec(
		`INSERT INTO settings(key, value) VALUES ('__retention_bypass', '0')
		 ON CONFLICT(key) DO UPDATE SET value='0'`,
	); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (db *DB) GetLogs(level, search string) ([]LogEntry, error) {
	return db.GetLogsFiltered(level, search, "")
}

// GetLogsFiltered extends GetLogs with the v0.1.10 risk_level column. Empty
// risk filters keep the prior behaviour. Recognised values: "low",
// "medium", "high"; anything else is ignored so a stale frontend bookmark
// with an unknown value doesn't break the query.
func (db *DB) GetLogsFiltered(level, search, risk string) ([]LogEntry, error) {
	query := `SELECT id, ts, level, message, request_id, risk_level FROM audit_log WHERE 1=1`
	args := []any{}
	if level != "" {
		query += ` AND level = ?`
		args = append(args, strings.ToUpper(level))
	}
	if search != "" {
		query += ` AND message LIKE ?`
		args = append(args, "%"+search+"%")
	}
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "low", "medium", "high":
		query += ` AND risk_level = ?`
		args = append(args, strings.ToLower(risk))
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
		if err := rows.Scan(&entry.ID, &entry.TS, &entry.Level, &entry.Message, &entry.RequestID, &entry.RiskLevel); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (db *DB) GetLogsForExport(level, search string, limit int) ([]LogEntry, error) {
	return db.GetLogsForExportFiltered(level, search, "", limit)
}

func (db *DB) GetLogsForExportFiltered(level, search, risk string, limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 100000
	}
	query := `SELECT id, ts, level, message, request_id, risk_level FROM audit_log WHERE 1=1`
	args := []any{}
	if level != "" {
		query += ` AND level = ?`
		args = append(args, strings.ToUpper(level))
	}
	if search != "" {
		query += ` AND message LIKE ?`
		args = append(args, "%"+search+"%")
	}
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "low", "medium", "high":
		query += ` AND risk_level = ?`
		args = append(args, strings.ToLower(risk))
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
		if err := rows.Scan(&entry.ID, &entry.TS, &entry.Level, &entry.Message, &entry.RequestID, &entry.RiskLevel); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// LoginState is the failure-counter + lockout-window row backing Q20's
// per-account lockout. LockedUntil is "" when the account is unlocked.
type LoginState struct {
	Username     string
	FailedCount  int
	LastFailedAt string
	LockedUntil  string
}

// Session is the server-side anchor for a logged-in operator (S5). The
// cookie carries only the id; everything authoritative lives in this
// row. RevokedAt == "" means active; non-empty means the operator (or
// a forced revoke) signed out.
type Session struct {
	ID         string
	Username   string
	CreatedAt  string
	LastSeenAt string
	ExpiresAt  string
	RevokedAt  string
}

// CreateSession inserts a fresh session row. id must be a
// cryptographically random opaque token chosen by the caller (the
// Login handler uses RandomSecret() which gives 32 bytes of entropy).
// expiresAt is RFC3339 UTC.
func (db *DB) CreateSession(id, username, expiresAt string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.sql.Exec(
		`INSERT INTO sessions(id, username, created_at, last_seen_at, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, username, now, now, expiresAt,
	)
	return err
}

// GetSession returns the row for id. Returns sql.ErrNoRows when the
// session does not exist — the RequireAuth middleware treats that as
// "invalid session, redirect to login".
func (db *DB) GetSession(id string) (Session, error) {
	var s Session
	err := db.sql.QueryRow(
		`SELECT id, username, created_at, last_seen_at, expires_at, revoked_at
		 FROM sessions WHERE id = ?`,
		id,
	).Scan(&s.ID, &s.Username, &s.CreatedAt, &s.LastSeenAt, &s.ExpiresAt, &s.RevokedAt)
	return s, err
}

// TouchSession updates last_seen_at to now. Called by RequireAuth on
// every successful auth check so an idle session ages out via its
// expires_at but an active one keeps moving forward. The expires_at
// itself is NOT bumped here — sliding-window expiry is operator-
// policy that lives in app code.
func (db *DB) TouchSession(id string) error {
	_, err := db.sql.Exec(
		`UPDATE sessions SET last_seen_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), id,
	)
	return err
}

// RevokeSession is the logout path: flip revoked_at on a specific id.
// RequireAuth refuses subsequent reuses of the cookie even though the
// cookie's MaxAge has not elapsed. Idempotent.
func (db *DB) RevokeSession(id string) error {
	_, err := db.sql.Exec(
		`UPDATE sessions SET revoked_at = ? WHERE id = ? AND revoked_at = ''`,
		time.Now().UTC().Format(time.RFC3339), id,
	)
	return err
}

// RevokeAllForUser is the bulk path used when a password rotates: every
// active session for the user is invalidated so a stolen cookie cannot
// outlive the operator's intent.
func (db *DB) RevokeAllForUser(username string) error {
	_, err := db.sql.Exec(
		`UPDATE sessions SET revoked_at = ? WHERE username = ? AND revoked_at = ''`,
		time.Now().UTC().Format(time.RFC3339), username,
	)
	return err
}

// PruneExpiredSessions removes rows whose expires_at is in the past.
// Called by a background sweeper; missing it would leave the table
// growing unboundedly on a server that gets many short-lived logins
// (e.g. an operator opening dashboards from multiple devices).
func (db *DB) PruneExpiredSessions() (int64, error) {
	res, err := db.sql.Exec(
		`DELETE FROM sessions WHERE expires_at < ?`,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// GetLoginState returns the persisted counter for username, or a zero
// value (no failures) if no row exists yet. The zero value is the
// "never tried" state, treated as unlocked.
func (db *DB) GetLoginState(username string) (LoginState, error) {
	var ls LoginState
	ls.Username = username
	err := db.sql.QueryRow(
		`SELECT failed_count, last_failed_at, locked_until FROM login_state WHERE username = ?`,
		username,
	).Scan(&ls.FailedCount, &ls.LastFailedAt, &ls.LockedUntil)
	if err == sql.ErrNoRows {
		return ls, nil
	}
	if err != nil {
		return LoginState{}, err
	}
	return ls, nil
}

// SetLoginState upserts the row keyed by username.
func (db *DB) SetLoginState(state LoginState) error {
	_, err := db.sql.Exec(
		`INSERT INTO login_state(username, failed_count, last_failed_at, locked_until)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(username) DO UPDATE SET
			failed_count = excluded.failed_count,
			last_failed_at = excluded.last_failed_at,
			locked_until = excluded.locked_until`,
		state.Username, state.FailedCount, state.LastFailedAt, state.LockedUntil,
	)
	return err
}

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
// this user" (the per-handler logic decides whether to block
// password-only login based on AppSettings.TOTPRequired).
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
