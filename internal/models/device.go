package models

type Device struct {
	MAC                string   `json:"mac"`
	IP                 string   `json:"ip"`
	Name               string   `json:"name"`
	Model              string   `json:"model"`
	FW                 string   `json:"fw"`
	Gen                int      `json:"gen"`
	Online             bool     `json:"online"`
	LastSeen           string   `json:"last_seen"`
	LastRefreshAttempt string   `json:"last_refresh_attempt"`
	LastRefreshOK      bool     `json:"last_refresh_ok"`
	LastRefreshError   string   `json:"last_refresh_error"`
	FirstSeen          string   `json:"first_seen"`
	DeviceNum          int      `json:"device_num"`
	ConsecutiveMisses  int      `json:"consecutive_misses"`
	MQTTEnabled        *bool    `json:"mqtt_enabled"`
	MQTTServer         string   `json:"mqtt_server"`
	MQTTClientID       string   `json:"mqtt_client_id"`
	MQTTTopicPrefix    string   `json:"mqtt_topic_prefix"`
	MQTTFlagsNA        string   `json:"mqtt_flags_na"`
	Lat                *float64 `json:"lat"`
	Lon                *float64 `json:"lon"`
	TZ                 string   `json:"tz"`
	SNTPServer         string   `json:"sntp_server"`
	WSEnabled          *bool    `json:"ws_enabled"`
	WSServer           string   `json:"ws_server"`
	WSConnected        bool     `json:"ws_connected"`
	BLEGWEnabled       *bool    `json:"ble_gw_enabled"`
	BLERPCEnabled      *bool    `json:"ble_rpc_enabled"`
	BLEObserverEnabled *bool    `json:"ble_observer_enabled"`
	WiFiSSID           string   `json:"wifi_ssid"`
	CloudEnabled       *bool    `json:"cloud_enabled"`
	CloudConnected     bool     `json:"cloud_connected"`
	MQTTConnected      bool     `json:"mqtt_connected"`
	MatterEnabled      *bool    `json:"matter_enabled"`
	EcoMode            *bool    `json:"eco_mode"`
	Discoverable       *bool    `json:"discoverable"`
	AuthRequired       bool     `json:"auth_required"`
	AuthError          string   `json:"auth_error"`
	AuthLockedUntil    string   `json:"auth_locked_until"`
	Scheme             string   `json:"scheme"`
	EnhancedSecurity   *bool    `json:"enhanced_security"`
	TLSCertValid       *bool    `json:"tls_cert_valid"`
	TLSAllowInsecure   *bool    `json:"tls_allow_insecure"`
	WiFiHostname       string   `json:"wifi_hostname"`
	WiFiChannel        int      `json:"wifi_channel"`
	// Live power-monitoring readings, summed across EM/EM1/PM1/Switch
	// components. Pointer means "device doesn't expose any power telemetry";
	// zero is a valid reading (e.g. switch off).
	PowerW            *float64 `json:"power_w"`
	VoltageV          *float64 `json:"voltage_v"`
	CurrentA          *float64 `json:"current_a"`
	FWAvailableStable string   `json:"fw_available_stable"`
	FWAvailableBeta   string   `json:"fw_available_beta"`
	FWCheckedAt       string   `json:"fw_checked_at"`
	Serial            string   `json:"serial"`
	Compliant         bool     `json:"compliant"`
	ComplianceIssues  []string `json:"compliance_issues"`
	RawConfig         string   `json:"-"`
	RawStatus         string   `json:"-"`
}
