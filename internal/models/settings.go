package models

import "strings"

type AppSettings struct {
	Subnets             []string `json:"subnets"`
	ScanTimeout         float64  `json:"scan_timeout"`
	RefreshTimeout      float64  `json:"refresh_timeout"`
	ScanConcurrency     int      `json:"scan_concurrency"`
	EnableMDNS          bool     `json:"enable_mdns"`
	AdvancedModeEnabled bool     `json:"advanced_mode_enabled"`
	Gen2BadgeClass      string   `json:"gen2_badge_class"`
	Gen3BadgeClass      string   `json:"gen3_badge_class"`
	Gen4BadgeClass      string   `json:"gen4_badge_class"`
	// FirmwareInstallTimeout caps how long an individual device may take to
	// reboot onto the new firmware before the install_job marks it "unknown".
	// Seconds; default 300 (5 min). Per-device, not job-total.
	FirmwareInstallTimeout float64 `json:"firmware_install_timeout"`
	// FirmwareInstallPollInterval is how often the install_job re-queries a
	// device's reported firmware version while waiting for the reboot. Seconds;
	// default 5. Bounded to [1, 60] in Normalize — too aggressive hammers slow
	// devices, too slow makes the UI feel stuck.
	FirmwareInstallPollInterval float64 `json:"firmware_install_poll_interval"`
	// FirmwareCheckInterval triggers a periodic firmware_check job at the
	// given cadence in seconds. 0 disables the scheduler (manual-only). The
	// scheduler skips a tick if a firmware_check is already running.
	FirmwareCheckInterval int             `json:"firmware_check_interval"`
	Compliance            ComplianceRules `json:"compliance"`
	// MCPEnabled controls whether the read-only MCP server starts at the
	// next container restart, when no SHELLYADMIN_MCP_TOKEN env var is set.
	// Env var takes precedence over this flag (see ADR-0011).
	MCPEnabled bool `json:"mcp_enabled,omitempty"`
	// MCPToken is the bearer/path token used to authenticate MCP clients.
	// Stored encrypted at rest via internal/core/secretbox; the API GET
	// path redacts it to "<set>" / "" so plaintext never leaves the
	// process. Ignored when SHELLYADMIN_MCP_TOKEN env var is set.
	MCPToken string `json:"mcp_token,omitempty"`
	// MCPManagedByEnv is a read-only flag the API GET handler sets to
	// true when SHELLYADMIN_MCP_TOKEN is present in the environment.
	// Tells the UI to disable the MCP fields and show an override notice.
	// Never persisted.
	MCPManagedByEnv bool `json:"mcp_managed_by_env,omitempty"`
	// MCPRunning is a read-only flag the API GET handler sets to true
	// when an MCP listener goroutine is currently active. The UI uses
	// it for a live status badge (Running / Stopped) on the MCP card.
	// Never persisted.
	MCPRunning bool `json:"mcp_running,omitempty"`
	// AuditRetentionDays caps how long audit_log rows are kept before
	// the background pruner removes them. 0 disables pruning (rows are
	// kept indefinitely). Default 90 days; clamped to [0, 3650] in
	// Normalize. The pruner runs hourly and uses the audit_log
	// append-only trigger's controlled bypass to actually delete rows.
	AuditRetentionDays int `json:"audit_retention_days,omitempty"`
	// AutoBackupEnabled toggles the background SQLite snapshot job.
	// When true, every AutoBackupIntervalHours the service writes
	// shellyctl.db.snap-<unix>.sqlite into the data directory via
	// `VACUUM INTO` (atomic, online-safe), keeping only
	// AutoBackupKeep most recent files. Encryption-key file is NOT
	// snapshotted — that is a deliberate operator step.
	AutoBackupEnabled       bool `json:"auto_backup_enabled,omitempty"`
	AutoBackupIntervalHours int  `json:"auto_backup_interval_hours,omitempty"`
	AutoBackupKeep          int  `json:"auto_backup_keep,omitempty"`

	// AuditWebhookURL is the optional off-host audit sink (T11). When
	// non-empty, every audit_log row written through LogCtx is also
	// POSTed as JSON to this URL on a best-effort, fire-and-forget
	// basis. A webhook delivery failure does NOT block the audit-log
	// row from being persisted locally — the local DB is still the
	// authoritative trail, the webhook is the replica.
	AuditWebhookURL string `json:"audit_webhook_url,omitempty"`
	// AuditWebhookMinLevel filters which rows are forwarded
	// ("DEBUG" / "INFO" / "WARN" / "ERROR"; empty = INFO and above).
	// A noisy DEBUG-forwarder would saturate a low-throughput sink
	// (Slack, Discord) on every refresh tick.
	AuditWebhookMinLevel string `json:"audit_webhook_min_level,omitempty"`
}

type ComplianceRules struct {
	WiFiSSID        string   `json:"wifi_ssid"`
	MQTTEnabled     *bool    `json:"mqtt_enabled"`
	MQTTServer      string   `json:"mqtt_server"`
	MQTTClientID    string   `json:"mqtt_client_id"`
	MQTTTopicPrefix string   `json:"mqtt_topic_prefix"`
	MQTTRPCNtf      *bool    `json:"mqtt_rpc_ntf"`
	MQTTStatusNtf   *bool    `json:"mqtt_status_ntf"`
	MQTTEnableRPC   *bool    `json:"mqtt_enable_rpc"`
	MQTTEnableCtrl  *bool    `json:"mqtt_enable_control"`
	MQTTConnected   *bool    `json:"mqtt_connected"`
	CloudEnabled    *bool    `json:"cloud_enabled"`
	CloudConnected  *bool    `json:"cloud_connected"`
	WSEnabled       *bool    `json:"ws_enabled"`
	WSConnected     *bool    `json:"ws_connected"`
	WSServer        string   `json:"ws_server"`
	WSTLSMode       string   `json:"ws_tls_mode"`
	WSSSLCa         string   `json:"ws_ssl_ca"`
	BLEGWEnabled    *bool    `json:"ble_gw_enabled"`
	BLERPCEnabled   *bool    `json:"ble_rpc_enable"`
	BLEObserver     *bool    `json:"ble_observer_enable"`
	TZ              string   `json:"tz"`
	SNTPServer      string   `json:"sntp_server"`
	Lat             *float64 `json:"lat"`
	Lon             *float64 `json:"lon"`
	DebugWebSocket  *bool    `json:"sys_debug_websocket"`
	DebugUDPHost    string   `json:"sys_debug_udp_host"`
	RPCUDPPort      *int     `json:"sys_rpc_udp_port"`
	EcoMode         *bool    `json:"eco_mode"`
	Discoverable    *bool    `json:"discoverable"`
	WiFiAPEnabled   *bool    `json:"wifi_ap_enabled"`
	WiFiAPIsOpen    *bool    `json:"wifi_ap_is_open"`
	EthEnabled      *bool    `json:"eth_enabled"`
	EthIPv4Mode     string   `json:"eth_ipv4mode"`
	DebugMQTT       *bool    `json:"sys_debug_mqtt"`
	MatterEnabled   *bool    `json:"matter_enabled"`
	ModbusEnabled   *bool    `json:"modbus_enabled"`
	ZigbeeEnabled   *bool    `json:"zigbee_enabled"`
	// Firmware 2.0.0-beta1 additions:
	EnhancedSecurity *bool        `json:"enhanced_security"`
	TLSCertValid     *bool        `json:"tls_cert_valid"`
	WiFiHostname     string       `json:"wifi_hostname"`
	BLEPaired        *bool        `json:"ble_paired"`
	WebhooksConfig   *bool        `json:"webhooks_configured"`
	AutoUpdateStage  string       `json:"auto_update_stage"` // "" (skip) | "off" | "stable" | "beta"
	CustomRules      []CustomRule `json:"custom_rules"`
}

type CustomRule struct {
	Label  string `json:"label"`
	Source string `json:"source"`
	Path   string `json:"path"`
	Op     string `json:"op"`
	Value  string `json:"value"`
	GenMin int    `json:"gen_min"`
	GenMax int    `json:"gen_max"`
}

func DefaultSettings() AppSettings {
	return AppSettings{
		Subnets:                     []string{},
		ScanTimeout:                 2,
		RefreshTimeout:              5,
		ScanConcurrency:             64,
		EnableMDNS:                  false,
		Gen2BadgeClass:              "bg-warning text-dark",
		Gen3BadgeClass:              "bg-success",
		Gen4BadgeClass:              "bg-info text-dark",
		FirmwareInstallTimeout:      300,
		FirmwareInstallPollInterval: 5,
		FirmwareCheckInterval:       0,
		AuditRetentionDays:          90,
		AutoBackupEnabled:           false,
		AutoBackupIntervalHours:     24,
		AutoBackupKeep:              7,
	}
}

func (s *AppSettings) Normalize() {
	cleaned := make([]string, 0, len(s.Subnets))
	for _, subnet := range s.Subnets {
		subnet = strings.TrimSpace(subnet)
		if subnet != "" {
			cleaned = append(cleaned, subnet)
		}
	}
	s.Subnets = cleaned
	if s.ScanConcurrency <= 0 {
		s.ScanConcurrency = 64
	}
	if s.ScanTimeout <= 0 {
		s.ScanTimeout = 2
	}
	if s.RefreshTimeout <= 0 {
		s.RefreshTimeout = 5
	}
	if strings.TrimSpace(s.Gen2BadgeClass) == "" {
		s.Gen2BadgeClass = "bg-warning text-dark"
	}
	if strings.TrimSpace(s.Gen3BadgeClass) == "" {
		s.Gen3BadgeClass = "bg-success"
	}
	if strings.TrimSpace(s.Gen4BadgeClass) == "" {
		s.Gen4BadgeClass = "bg-info text-dark"
	}
	if s.FirmwareInstallTimeout <= 0 {
		s.FirmwareInstallTimeout = 300
	}
	if s.FirmwareInstallPollInterval <= 0 {
		s.FirmwareInstallPollInterval = 5
	} else if s.FirmwareInstallPollInterval < 1 {
		s.FirmwareInstallPollInterval = 1
	} else if s.FirmwareInstallPollInterval > 60 {
		s.FirmwareInstallPollInterval = 60
	}
	if s.FirmwareCheckInterval < 0 {
		s.FirmwareCheckInterval = 0
	}
	// Audit retention: 0 disables, otherwise clamp to [1, 3650] (10 years).
	if s.AuditRetentionDays < 0 {
		s.AuditRetentionDays = 0
	} else if s.AuditRetentionDays > 3650 {
		s.AuditRetentionDays = 3650
	}
	// Auto-backup bounds. Interval [1, 168]h (hourly to weekly); keep
	// [1, 100] snapshots.
	if s.AutoBackupIntervalHours <= 0 {
		s.AutoBackupIntervalHours = 24
	} else if s.AutoBackupIntervalHours > 168 {
		s.AutoBackupIntervalHours = 168
	}
	if s.AutoBackupKeep <= 0 {
		s.AutoBackupKeep = 7
	} else if s.AutoBackupKeep > 100 {
		s.AutoBackupKeep = 100
	}
	s.MCPToken = strings.TrimSpace(s.MCPToken)
	// MCPManagedByEnv and MCPRunning are runtime overlays set by the API
	// layer at GET time; never persist them.
	s.MCPManagedByEnv = false
	s.MCPRunning = false
	s.Compliance.Normalize()
}

func (c *ComplianceRules) Normalize() {
	c.WSTLSMode = normalizeWSTLSMode(c.WSTLSMode)
	if c.WSTLSMode != "user" {
		c.WSSSLCa = ""
	}
	c.WSServer = strings.TrimSpace(c.WSServer)
	c.TZ = strings.TrimSpace(c.TZ)
	c.SNTPServer = strings.TrimSpace(c.SNTPServer)
	c.DebugUDPHost = strings.TrimSpace(c.DebugUDPHost)
	if c.RPCUDPPort != nil && *c.RPCUDPPort < 0 {
		zero := 0
		c.RPCUDPPort = &zero
	}
	c.EthIPv4Mode = normalizeEthIPv4Mode(c.EthIPv4Mode)
	c.WiFiHostname = strings.TrimSpace(c.WiFiHostname)
}

func normalizeEthIPv4Mode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "dhcp", "static":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func normalizeWSTLSMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "no_validation", "default", "user":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}
