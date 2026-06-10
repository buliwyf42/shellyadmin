package db

// Device inventory persistence. MOVED FROM db.go — db-layer split by domain
// (post-v0.5.2 review item 6); bodies unchanged.

import (
	"database/sql"
	"encoding/json"

	"shellyadmin/internal/models"
)

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
