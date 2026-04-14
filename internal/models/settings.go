package models

import "strings"

type AppSettings struct {
	Subnets         []string        `json:"subnets"`
	ScanTimeout     float64         `json:"scan_timeout"`
	RefreshTimeout  float64         `json:"refresh_timeout"`
	ScanConcurrency int             `json:"scan_concurrency"`
	Compliance      ComplianceRules `json:"compliance"`
}

type ComplianceRules struct {
	WiFiSSID        string       `json:"wifi_ssid"`
	MQTTEnabled     *bool        `json:"mqtt_enabled"`
	MQTTServer      string       `json:"mqtt_server"`
	MQTTClientID    string       `json:"mqtt_client_id"`
	MQTTTopicPrefix string       `json:"mqtt_topic_prefix"`
	MQTTRPCNtf      *bool        `json:"mqtt_rpc_ntf"`
	MQTTStatusNtf   *bool        `json:"mqtt_status_ntf"`
	MQTTEnableRPC   *bool        `json:"mqtt_enable_rpc"`
	MQTTEnableCtrl  *bool        `json:"mqtt_enable_control"`
	CloudConnected  *bool        `json:"cloud_connected"`
	WSEnabled       *bool        `json:"ws_enabled"`
	WSConnected     *bool        `json:"ws_connected"`
	WSServer        string       `json:"ws_server"`
	WSSSLCa         string       `json:"ws_ssl_ca"`
	BLEGWEnabled    *bool        `json:"ble_gw_enabled"`
	BLERPCEnabled   *bool        `json:"ble_rpc_enable"`
	TZ              string       `json:"tz"`
	SNTPServer      string       `json:"sntp_server"`
	Lat             *float64     `json:"lat"`
	Lon             *float64     `json:"lon"`
	TimeFormat      string       `json:"time_format"`
	EcoMode         *bool        `json:"eco_mode"`
	Discoverable    *bool        `json:"discoverable"`
	CustomRules     []CustomRule `json:"custom_rules"`
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
		Subnets:         []string{},
		ScanTimeout:     2,
		RefreshTimeout:  5,
		ScanConcurrency: 64,
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
}
