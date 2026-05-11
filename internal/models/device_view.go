package models

// DeviceListView is the slim per-device shape returned by /api/devices and
// the MCP list_devices tool. It drops fields no SPA list page or MCP
// list-tool consumer references:
//
//   - SupportedMethods — the full Shelly.ListMethods cache (~1-2 KB per
//     device). Used only internally for action-eligibility filtering;
//     callers needing it should use GetDeviceDetail / get_device.
//   - Batch, FWID — Shelly.GetDeviceInfo extras. Only DeviceDetail.svelte
//     uses them; not visible on the Devices table.
//   - ConsecutiveMisses — refresh-loop internal counter.
//   - MQTTFlagsNA — unused in any SPA file.
//
// Everything else mirrors models.Device 1:1 so the SPA's existing
// per-row table + popovers continue to work unchanged.
//
// ADDED in v0.3.0 (M8, docs/plans/phase-4b-refactor-block.md Block 4b.2).
// The Go service layer continues to use models.Device everywhere;
// projection to DeviceListView happens at the API handler + MCP tool
// boundary via Device.ToListView().
type DeviceListView struct {
	MAC                string   `json:"mac"`
	IP                 string   `json:"ip"`
	Name               string   `json:"name"`
	Model              string   `json:"model"`
	App                string   `json:"app"`
	FW                 string   `json:"fw"`
	Gen                int      `json:"gen"`
	Online             bool     `json:"online"`
	LastSeen           string   `json:"last_seen"`
	LastRefreshAttempt string   `json:"last_refresh_attempt"`
	LastRefreshOK      bool     `json:"last_refresh_ok"`
	LastRefreshError   string   `json:"last_refresh_error"`
	FirstSeen          string   `json:"first_seen"`
	DeviceNum          int      `json:"device_num"`
	MQTTEnabled        *bool    `json:"mqtt_enabled"`
	MQTTServer         string   `json:"mqtt_server"`
	MQTTClientID       string   `json:"mqtt_client_id"`
	MQTTTopicPrefix    string   `json:"mqtt_topic_prefix"`
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
	PowerW             *float64 `json:"power_w"`
	VoltageV           *float64 `json:"voltage_v"`
	CurrentA           *float64 `json:"current_a"`
	FWAvailableStable  string   `json:"fw_available_stable"`
	FWAvailableBeta    string   `json:"fw_available_beta"`
	FWCheckedAt        string   `json:"fw_checked_at"`
	FWAutoUpdate       string   `json:"fw_auto_update"`
	Serial             string   `json:"serial"`
	Compliant          bool     `json:"compliant"`
	ComplianceIssues   []string `json:"compliance_issues"`
	SwitchCount        int      `json:"switch_count"`
	CoverCount         int      `json:"cover_count"`
	LightCount         int      `json:"light_count"`
}

// ToListView projects a full Device down to the slim DeviceListView
// shape returned by /api/devices and the MCP list_devices tool.
func (d Device) ToListView() DeviceListView {
	return DeviceListView{
		MAC:                d.MAC,
		IP:                 d.IP,
		Name:               d.Name,
		Model:              d.Model,
		App:                d.App,
		FW:                 d.FW,
		Gen:                d.Gen,
		Online:             d.Online,
		LastSeen:           d.LastSeen,
		LastRefreshAttempt: d.LastRefreshAttempt,
		LastRefreshOK:      d.LastRefreshOK,
		LastRefreshError:   d.LastRefreshError,
		FirstSeen:          d.FirstSeen,
		DeviceNum:          d.DeviceNum,
		MQTTEnabled:        d.MQTTEnabled,
		MQTTServer:         d.MQTTServer,
		MQTTClientID:       d.MQTTClientID,
		MQTTTopicPrefix:    d.MQTTTopicPrefix,
		Lat:                d.Lat,
		Lon:                d.Lon,
		TZ:                 d.TZ,
		SNTPServer:         d.SNTPServer,
		WSEnabled:          d.WSEnabled,
		WSServer:           d.WSServer,
		WSConnected:        d.WSConnected,
		BLEGWEnabled:       d.BLEGWEnabled,
		BLERPCEnabled:      d.BLERPCEnabled,
		BLEObserverEnabled: d.BLEObserverEnabled,
		WiFiSSID:           d.WiFiSSID,
		CloudEnabled:       d.CloudEnabled,
		CloudConnected:     d.CloudConnected,
		MQTTConnected:      d.MQTTConnected,
		MatterEnabled:      d.MatterEnabled,
		EcoMode:            d.EcoMode,
		Discoverable:       d.Discoverable,
		AuthRequired:       d.AuthRequired,
		AuthError:          d.AuthError,
		AuthLockedUntil:    d.AuthLockedUntil,
		Scheme:             d.Scheme,
		EnhancedSecurity:   d.EnhancedSecurity,
		TLSCertValid:       d.TLSCertValid,
		TLSAllowInsecure:   d.TLSAllowInsecure,
		WiFiHostname:       d.WiFiHostname,
		WiFiChannel:        d.WiFiChannel,
		PowerW:             d.PowerW,
		VoltageV:           d.VoltageV,
		CurrentA:           d.CurrentA,
		FWAvailableStable:  d.FWAvailableStable,
		FWAvailableBeta:    d.FWAvailableBeta,
		FWCheckedAt:        d.FWCheckedAt,
		FWAutoUpdate:       d.FWAutoUpdate,
		Serial:             d.Serial,
		Compliant:          d.Compliant,
		ComplianceIssues:   d.ComplianceIssues,
		SwitchCount:        d.SwitchCount,
		CoverCount:         d.CoverCount,
		LightCount:         d.LightCount,
	}
}

// ToListViews maps a slice of Device to DeviceListView in place. Convenience
// wrapper around ToListView for the API handler + MCP list_devices boundary.
func ToListViews(devices []Device) []DeviceListView {
	out := make([]DeviceListView, len(devices))
	for i, d := range devices {
		out[i] = d.ToListView()
	}
	return out
}
