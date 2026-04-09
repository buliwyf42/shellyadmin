package models

type Device struct {
	MAC               string   `json:"mac"`
	IP                string   `json:"ip"`
	Name              string   `json:"name"`
	Model             string   `json:"model"`
	FW                string   `json:"fw"`
	Gen               int      `json:"gen"`
	Online            bool     `json:"online"`
	LastSeen          string   `json:"last_seen"`
	FirstSeen         string   `json:"first_seen"`
	DeviceNum         int      `json:"device_num"`
	ConsecutiveMisses int      `json:"consecutive_misses"`
	MQTTEnabled       *bool    `json:"mqtt_enabled"`
	MQTTServer        string   `json:"mqtt_server"`
	MQTTClientID      string   `json:"mqtt_client_id"`
	MQTTTopicPrefix   string   `json:"mqtt_topic_prefix"`
	MQTTFlagsNA       string   `json:"mqtt_flags_na"`
	Lat               *float64 `json:"lat"`
	Lon               *float64 `json:"lon"`
	TZ                string   `json:"tz"`
	TimeFormat        string   `json:"time_format"`
	SNTPServer        string   `json:"sntp_server"`
	WSEnabled         *bool    `json:"ws_enabled"`
	WSServer          string   `json:"ws_server"`
	WSConnected       bool     `json:"ws_connected"`
	BLEGWEnabled      *bool    `json:"ble_gw_enabled"`
	WiFiSSID          string   `json:"wifi_ssid"`
	CloudEnabled      *bool    `json:"cloud_enabled"`
	CloudConnected    bool     `json:"cloud_connected"`
	MatterEnabled     *bool    `json:"matter_enabled"`
	EcoMode           *bool    `json:"eco_mode"`
	Discoverable      *bool    `json:"discoverable"`
	FWStatus          string   `json:"fw_status"`
	FWAvailableVer    string   `json:"fw_available_ver"`
	Serial            string   `json:"serial"`
	Compliant         bool     `json:"compliant"`
	ComplianceIssues  []string `json:"compliance_issues"`
	RawConfig         string   `json:"-"`
	RawStatus         string   `json:"-"`
}
