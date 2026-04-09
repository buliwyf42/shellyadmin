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

	"shellyadmin/internal/models"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type DB struct {
	sql *sql.DB
}

type LogEntry struct {
	ID      int    `json:"id"`
	TS      string `json:"ts"`
	Level   string `json:"level"`
	Message string `json:"message"`
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
	return db, nil
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
		consecutive_misses, mqtt_enabled, mqtt_server, mqtt_client_id, mqtt_topic_prefix, mqtt_flags_na,
		lat, lon, tz, ws_enabled, ws_server, ble_gw_enabled, wifi_ssid, fw_status, fw_available_ver,
		cloud_enabled, cloud_connected, ws_connected, matter_enabled, time_format, sntp_server, serial,
		eco_mode, discoverable, raw_config, raw_status
		FROM devices ORDER BY device_num ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Device
	for rows.Next() {
		var d models.Device
		var online, cloudConnected, wsConnected int
		if err := rows.Scan(&d.MAC, &d.IP, &d.Name, &d.Model, &d.FW, &d.Gen, &online, &d.LastSeen, &d.FirstSeen, &d.DeviceNum,
			&d.ConsecutiveMisses, &d.MQTTEnabled, &d.MQTTServer, &d.MQTTClientID, &d.MQTTTopicPrefix, &d.MQTTFlagsNA,
			&d.Lat, &d.Lon, &d.TZ, &d.WSEnabled, &d.WSServer, &d.BLEGWEnabled, &d.WiFiSSID, &d.FWStatus, &d.FWAvailableVer,
			&d.CloudEnabled, &cloudConnected, &wsConnected, &d.MatterEnabled, &d.TimeFormat, &d.SNTPServer, &d.Serial,
			&d.EcoMode, &d.Discoverable, &d.RawConfig, &d.RawStatus); err != nil {
			return nil, err
		}
		d.Online = online == 1
		d.CloudConnected = cloudConnected == 1
		d.WSConnected = wsConnected == 1
		out = append(out, d)
	}
	return out, rows.Err()
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
		d.ConsecutiveMisses = 0
		d.Online = true
		if d.FWStatus == "" {
			d.FWStatus = "unknown"
		}
		if err := db.upsertDevice(d); err != nil {
			return err
		}
	}
	for _, d := range existing {
		if seen[d.MAC] {
			continue
		}
		d.ConsecutiveMisses++
		if d.ConsecutiveMisses >= 2 {
			d.Online = false
		}
		if err := db.upsertDevice(d); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) upsertDevice(d models.Device) error {
	_, err := db.sql.Exec(`INSERT INTO devices (
		mac, ip, name, model, fw, gen, online, last_seen, first_seen, device_num, consecutive_misses,
		mqtt_enabled, mqtt_server, mqtt_client_id, mqtt_topic_prefix, mqtt_flags_na, lat, lon, tz,
		ws_enabled, ws_server, ble_gw_enabled, wifi_ssid, fw_status, fw_available_ver, cloud_enabled,
		cloud_connected, ws_connected, matter_enabled, time_format, sntp_server, serial, eco_mode, discoverable, raw_config, raw_status
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(mac) DO UPDATE SET
		ip=excluded.ip, name=excluded.name, model=excluded.model, fw=excluded.fw, gen=excluded.gen,
		online=excluded.online, last_seen=excluded.last_seen, first_seen=excluded.first_seen,
		device_num=excluded.device_num, consecutive_misses=excluded.consecutive_misses,
		mqtt_enabled=excluded.mqtt_enabled, mqtt_server=excluded.mqtt_server,
		mqtt_client_id=excluded.mqtt_client_id, mqtt_topic_prefix=excluded.mqtt_topic_prefix,
		mqtt_flags_na=excluded.mqtt_flags_na, lat=excluded.lat, lon=excluded.lon, tz=excluded.tz,
		ws_enabled=excluded.ws_enabled, ws_server=excluded.ws_server, ble_gw_enabled=excluded.ble_gw_enabled,
		wifi_ssid=excluded.wifi_ssid, fw_status=excluded.fw_status, fw_available_ver=excluded.fw_available_ver,
		cloud_enabled=excluded.cloud_enabled, cloud_connected=excluded.cloud_connected,
		ws_connected=excluded.ws_connected, matter_enabled=excluded.matter_enabled,
		time_format=excluded.time_format, sntp_server=excluded.sntp_server, serial=excluded.serial,
		eco_mode=excluded.eco_mode, discoverable=excluded.discoverable,
		raw_config=excluded.raw_config, raw_status=excluded.raw_status`,
		d.MAC, d.IP, d.Name, d.Model, d.FW, d.Gen, boolToInt(d.Online), d.LastSeen, d.FirstSeen, d.DeviceNum, d.ConsecutiveMisses,
		d.MQTTEnabled, d.MQTTServer, d.MQTTClientID, d.MQTTTopicPrefix, d.MQTTFlagsNA, d.Lat, d.Lon, d.TZ,
		d.WSEnabled, d.WSServer, d.BLEGWEnabled, d.WiFiSSID, d.FWStatus, d.FWAvailableVer, d.CloudEnabled,
		boolToInt(d.CloudConnected), boolToInt(d.WSConnected), d.MatterEnabled, d.TimeFormat, d.SNTPServer, d.Serial, d.EcoMode, d.Discoverable, d.RawConfig, d.RawStatus)
	return err
}

func (db *DB) ForgetDevice(target string) error {
	_, err := db.sql.Exec(`DELETE FROM devices WHERE mac = ? OR ip = ?`, target, target)
	return err
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

func (db *DB) CompleteJob(id int64, status, result, errText string, done, total int) error {
	_, err := db.sql.Exec(`UPDATE jobs
		SET status = ?, result = ?, error = ?, done = ?, total = ?, updated_at = ?
		WHERE id = ?`, status, result, errText, done, total, now(), id)
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

func (db *DB) GetTemplate(name string) (string, error) {
	var content string
	err := db.sql.QueryRow(`SELECT content FROM templates WHERE name = ?`, name).Scan(&content)
	return content, err
}

func (db *DB) SaveTemplate(name, content string) error {
	_, err := db.sql.Exec(`INSERT INTO templates(name, content, created_at) VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET content=excluded.content`, name, content, now())
	return err
}

func (db *DB) DeleteTemplate(name string) error {
	_, err := db.sql.Exec(`DELETE FROM templates WHERE name = ?`, name)
	return err
}

func (db *DB) AddLog(level, message string) error {
	_, err := db.sql.Exec(`INSERT INTO audit_log(ts, level, message) VALUES (?, ?, ?)`, now(), level, message)
	return err
}

func (db *DB) GetLogs(level, search string) ([]LogEntry, error) {
	query := `SELECT id, ts, level, message FROM audit_log WHERE 1=1`
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
		if err := rows.Scan(&entry.ID, &entry.TS, &entry.Level, &entry.Message); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
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
