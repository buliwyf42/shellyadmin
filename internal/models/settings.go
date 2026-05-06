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
	// FirmwareCheckInterval triggers a periodic firmware_check job at the
	// given cadence in seconds. 0 disables the scheduler (manual-only). The
	// scheduler skips a tick if a firmware_check is already running.
	FirmwareCheckInterval int             `json:"firmware_check_interval"`
	Compliance            ComplianceRules `json:"compliance"`
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
		Subnets:                []string{},
		ScanTimeout:            2,
		RefreshTimeout:         5,
		ScanConcurrency:        64,
		EnableMDNS:             false,
		Gen2BadgeClass:         "bg-warning text-dark",
		Gen3BadgeClass:         "bg-success",
		Gen4BadgeClass:         "bg-info text-dark",
		FirmwareInstallTimeout: 300,
		FirmwareCheckInterval:  0,
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
	if s.FirmwareCheckInterval < 0 {
		s.FirmwareCheckInterval = 0
	}
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
