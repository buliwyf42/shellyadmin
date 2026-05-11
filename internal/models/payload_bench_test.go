package models

import (
	"encoding/json"
	"testing"
)

// Synthetic 50-device fleet to measure /api/devices payload size before
// (full Device) vs. after (DeviceListView) the M8 projection split.
func TestPayloadSizeReductionListView(t *testing.T) {
	supported := []string{
		"Shelly.GetStatus", "Shelly.GetConfig", "Shelly.GetDeviceInfo", "Shelly.SetAuth",
		"Shelly.Update", "Shelly.CheckForUpdate", "Shelly.GetMethodSurface", "Shelly.ListMethods",
		"Shelly.PutUserCA", "Shelly.PutTLSClientCert", "Shelly.PutTLSClientKey",
		"KVS.Set", "KVS.Delete", "KVS.List", "KVS.Get", "KVS.GetMany",
		"Sys.GetStatus", "Sys.GetConfig", "Sys.SetConfig",
		"Schedule.Create", "Schedule.Delete", "Schedule.DeleteAll", "Schedule.List", "Schedule.Update",
		"WiFi.GetStatus", "WiFi.GetConfig", "WiFi.SetConfig", "WiFi.ListAPClients", "WiFi.Scan",
		"Eth.GetConfig", "Eth.SetConfig", "BLE.GetStatus", "BLE.GetConfig", "BLE.SetConfig",
		"Cloud.GetStatus", "Cloud.GetConfig", "Cloud.SetConfig", "Cloud.ListLogs",
		"MQTT.GetStatus", "MQTT.GetConfig", "MQTT.SetConfig",
		"WS.GetStatus", "WS.GetConfig", "WS.SetConfig",
		"Switch.SetConfig", "Switch.GetConfig", "Switch.GetStatus", "Switch.Set",
		"Switch.Toggle", "Switch.Recall", "Switch.ResetCounters", "Switch.ListGroups",
		"Cover.SetConfig", "Cover.GetConfig", "Cover.GetStatus",
		"Cover.Open", "Cover.Close", "Cover.Stop", "Cover.GoToPosition", "Cover.Calibrate", "Cover.SetPosition",
	}
	fleet := make([]Device, 50)
	for i := range fleet {
		fleet[i] = Device{
			MAC: "AA:BB:CC:DD:EE:00", IP: "192.168.1.10", Name: "device-00",
			Model: "SNSW-001P16EU", App: "Plus1PM",
			FW:    "20251220-114319/2.0.0-beta1-g7cce6c5",
			FWID:  "20251220-114319/2.0.0-beta1-g7cce6c5-shellies",
			Batch: "2430-Broadwell", Gen: 3, Online: true, Serial: "1234567890",
			SupportedMethods: supported,
			ComplianceIssues: []string{},
			MQTTServer:       "mqtt.home.lan", MQTTClientID: "shelly_xx", MQTTTopicPrefix: "shellies/",
			TZ: "Europe/Berlin", SNTPServer: "time.cloudflare.com",
			WiFiSSID: "homeNet5", WiFiHostname: "shelly-xx", WiFiChannel: 36,
			Scheme:            "https",
			FWAvailableStable: "2.0.0", FWAvailableBeta: "2.1.0-beta1",
			FWCheckedAt: "2026-05-11T13:00:00Z", FWAutoUpdate: "stable",
		}
	}
	full, _ := json.Marshal(fleet)
	slim, _ := json.Marshal(ToListViews(fleet))
	reduction := 100.0 * (1.0 - float64(len(slim))/float64(len(full)))
	t.Logf("full payload: %d bytes (%.1f KB)", len(full), float64(len(full))/1024)
	t.Logf("slim payload: %d bytes (%.1f KB)", len(slim), float64(len(slim))/1024)
	t.Logf("reduction:    %.1f%%", reduction)
	if reduction < 30 {
		t.Errorf("payload reduction = %.1f%%, want >= 30%% (M8 acceptance criterion)", reduction)
	}
}
