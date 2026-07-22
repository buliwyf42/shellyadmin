package models

type Device struct {
	MAC   string `json:"mac"`
	IP    string `json:"ip"`
	Name  string `json:"name"`
	Model string `json:"model"`
	// App is the device's short application code (e.g. "PlugSG3",
	// "Pro4PM"). Returned by Shelly under the "app" key on both GET
	// /shelly and Shelly.GetDeviceInfo. Friendlier than `Model` (which
	// is the canonical SKU) — the Devices/Firmware pages use it as the
	// primary "what is this" label and demote the SKU to the tooltip.
	App string `json:"app"`
	// Batch is the production batch label (e.g. "2430-Broadwell") from
	// Shelly.GetDeviceInfo. Useful for warranty / hardware-quirk
	// diagnostics. Empty until first firmware-check on devices that
	// pre-date this release.
	Batch string `json:"batch"`
	// FWID is the long firmware identifier from /shelly +
	// Shelly.GetDeviceInfo, including the build hash (e.g.
	// "20260423-102547/2.0.0-beta1-g8c7700a"). Distinct from `FW`
	// which is the user-friendly version string.
	FWID               string   `json:"fw_id"`
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
	FWAutoUpdate      string   `json:"fw_auto_update"`
	// SupportedMethods is the device's Shelly.ListMethods output, cached
	// at firmware-check / refresh time. Drives the per-device action
	// catalog filter — see ADR-0010. nil = "not yet probed".
	SupportedMethods []string `json:"supported_methods"`
	Serial           string   `json:"serial"`
	Compliant        bool     `json:"compliant"`
	ComplianceIssues []string `json:"compliance_issues"`
	// Component instance counts derived from RawStatus at GetDevices time
	// (not persisted). Used by the Devices-page Capabilities column to
	// show at-a-glance how many switch/cover/light instances each device
	// exposes. 0 = device doesn't expose this component type.
	SwitchCount int `json:"switch_count"`
	CoverCount  int `json:"cover_count"`
	LightCount  int `json:"light_count"`
	// FWAlt lists alternative firmware variants the device advertises under
	// Shelly.GetStatus → sys.alt (firmware 2.0.0+): a different protocol/OS
	// build for the same hardware — e.g. a Zigbee or Matter variant. Derived
	// from RawStatus at GetDevices time (not persisted). nil = none offered.
	// Read-only: switching to an alt variant is NOT wired — Shelly.Update
	// exposes no stage/url to select one (see CLAUDE.md firmware section).
	FWAlt []AltFirmwareVariant `json:"fw_alt,omitempty"`
	// Provisioning mirrors Shelly.GetStatus → sys.provisioning (secure-
	// provisioning state, firmware 2.0.0+). Derived from RawStatus; absent
	// until a device is enrolled in secure provisioning. nil = not present.
	Provisioning map[string]any `json:"provisioning,omitempty"`
	// FWFrozen: firmware.IsFeatureFrozen(Model) — derived at GetDevices time, not
	// persisted, same pattern as FWAlt/Provisioning. Informational only (ADR-0002).
	FWFrozen  bool   `json:"fw_frozen,omitempty"`
	RawConfig string `json:"-"`
	RawStatus string `json:"-"`
}

// AltFirmwareVariant describes one alternative firmware build a device can run
// (e.g. a Zigbee or Matter variant of the same hardware), flattened from the
// Shelly.GetStatus → sys.alt map. Stable/Beta hold the available version
// strings for that variant's channels (empty when the variant offers none).
type AltFirmwareVariant struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Desc   string `json:"desc"`
	Stable string `json:"stable,omitempty"`
	Beta   string `json:"beta,omitempty"`
}
