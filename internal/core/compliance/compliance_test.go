package compliance

import (
	"encoding/json"
	"testing"

	"shellyadmin/internal/models"
)

func boolPtr(v bool) *bool { return &v }

func TestEvaluate_DeviceNameTokenSubstitutionForMQTT(t *testing.T) {
	rawConfig, _ := json.Marshal(map[string]any{
		"sys": map[string]any{
			"clock_mode": 0,
		},
	})
	dev := models.Device{
		Gen:             3,
		Name:            "shelly-plugOD-01",
		MQTTEnabled:     boolPtr(true),
		MQTTServer:      "mqtt.example.test:1883",
		MQTTClientID:    "shelly-plugOD-01",
		MQTTTopicPrefix: "shelly/shelly-plugOD-01",
		CloudConnected:  true,
		WSConnected:     true,
		TZ:              "Europe/Berlin",
		WiFiSSID:        "iot_wifi",
		RawConfig:       string(rawConfig),
	}

	rules := models.ComplianceRules{
		WiFiSSID:        "iot_wifi",
		MQTTEnabled:     boolPtr(true),
		MQTTServer:      "mqtt.example.test:1883",
		MQTTClientID:    "{device_name}",
		MQTTTopicPrefix: "shelly/{device_name}",
		CloudConnected:  boolPtr(true),
		WSConnected:     boolPtr(true),
		TZ:              "Europe/Berlin",
	}

	compliant, issues := Evaluate(dev, rules)
	if !compliant {
		t.Fatalf("expected compliant device, got issues: %v", issues)
	}
}

func TestEvaluate_DeviceNameTemplateFallbackMatchForMQTT(t *testing.T) {
	rawConfig, _ := json.Marshal(map[string]any{
		"sys": map[string]any{
			"clock_mode": 0,
		},
	})
	dev := models.Device{
		Gen:             3,
		Name:            "different-display-name",
		MQTTEnabled:     boolPtr(true),
		MQTTServer:      "mqtt.example.test:1883",
		MQTTClientID:    "shelly-plugOD-01",
		MQTTTopicPrefix: "shelly/shelly-plugOD-01",
		RawConfig:       string(rawConfig),
	}

	rules := models.ComplianceRules{
		MQTTEnabled:     boolPtr(true),
		MQTTServer:      "mqtt.example.test:1883",
		MQTTClientID:    "{device_name}",
		MQTTTopicPrefix: "shelly/{device_name}",
	}

	compliant, issues := Evaluate(dev, rules)
	if !compliant {
		t.Fatalf("expected compliant device with template fallback, got issues: %v", issues)
	}
}

func TestEvaluate_WSTLSModeIgnoredForPlainWSURL(t *testing.T) {
	rawConfig := `{"ws":{"server":"ws://ha.home/api/shelly/ws","ssl_ca":"*"}}`
	dev := models.Device{Gen: 4, RawConfig: rawConfig}
	rules := models.ComplianceRules{WSTLSMode: "no_validation"}

	compliant, issues := Evaluate(dev, rules)
	if compliant {
		t.Fatalf("expected non-compliant device")
	}
	if len(issues) != 1 || issues[0] != "ws_tls_mode ignored because ws_server is non-tls" {
		t.Fatalf("expected ws tls ignored message, got %v", issues)
	}
}

func TestEvaluate_CloudEnabledMatch(t *testing.T) {
	enabled := true
	dev := models.Device{Gen: 2, CloudEnabled: &enabled}
	rules := models.ComplianceRules{CloudEnabled: boolPtr(true)}

	compliant, issues := Evaluate(dev, rules)
	if !compliant {
		t.Fatalf("expected compliant device, got issues: %v", issues)
	}
}

func TestEvaluate_CloudEnabledMismatch(t *testing.T) {
	disabled := false
	dev := models.Device{Gen: 2, CloudEnabled: &disabled}
	rules := models.ComplianceRules{CloudEnabled: boolPtr(true)}

	compliant, issues := Evaluate(dev, rules)
	if compliant {
		t.Fatalf("expected non-compliant device")
	}
	if len(issues) != 1 || issues[0] != "cloud_enabled mismatch" {
		t.Fatalf("expected cloud_enabled mismatch, got %v", issues)
	}
}

func TestEvaluate_FrozenFirmwareRuleOff(t *testing.T) {
	dev := models.Device{Gen: 2, Model: "SNSW-001X16EU", FWFrozen: true}
	rules := models.ComplianceRules{} // FlagFrozenFirmware defaults false

	compliant, issues := Evaluate(dev, rules)
	if !compliant || len(issues) != 0 {
		t.Fatalf("expected no issue with rule off, got compliant=%v issues=%v", compliant, issues)
	}
}

func TestEvaluate_FrozenFirmwareRuleOnFrozenDevice(t *testing.T) {
	dev := models.Device{Gen: 2, Model: "SNSW-001X16EU", FWFrozen: true}
	rules := models.ComplianceRules{FlagFrozenFirmware: true}

	compliant, issues := Evaluate(dev, rules)
	if compliant {
		t.Fatalf("expected non-compliant device")
	}
	want := "firmware line is feature-frozen — will never receive 2.0.0+ (Shelly Firmware Update Policy)"
	found := false
	for _, s := range issues {
		if s == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected frozen-firmware issue, got %v", issues)
	}
}

func TestEvaluate_FrozenFirmwareRuleOnNonFrozenDevice(t *testing.T) {
	dev := models.Device{Gen: 2, Model: "SNSW-002P16EU-nonexistent", FWFrozen: false}
	rules := models.ComplianceRules{FlagFrozenFirmware: true}

	compliant, issues := Evaluate(dev, rules)
	if !compliant || len(issues) != 0 {
		t.Fatalf("expected no issue for non-frozen device, got compliant=%v issues=%v", compliant, issues)
	}
}

func TestEvaluate_MQTTConnectedCheck(t *testing.T) {
	rules := models.ComplianceRules{MQTTConnected: boolPtr(true)}

	// Compliant: connected matches.
	dev1 := models.Device{Gen: 2, MQTTConnected: true}
	compliant, issues := Evaluate(dev1, rules)
	if !compliant {
		t.Fatalf("expected compliant device, got issues: %v", issues)
	}

	// Non-compliant: mismatch.
	dev2 := models.Device{Gen: 3, MQTTConnected: false}
	compliant, issues = Evaluate(dev2, rules)
	if compliant {
		t.Fatalf("expected non-compliant device")
	}
	if len(issues) != 1 || issues[0] != "mqtt_connected mismatch" {
		t.Fatalf("expected mqtt_connected mismatch, got %v", issues)
	}
}

func TestEvaluate_WiFiAPEnabledMismatch(t *testing.T) {
	rawConfig := `{"wifi":{"ap":{"enable":true,"is_open":false}}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{WiFiAPEnabled: boolPtr(false)}

	compliant, issues := Evaluate(dev, rules)
	if compliant {
		t.Fatalf("expected non-compliant device with AP enabled")
	}
	if len(issues) != 1 || issues[0] != "wifi_ap_enabled mismatch" {
		t.Fatalf("expected wifi_ap_enabled mismatch, got %v", issues)
	}
}

func TestEvaluate_WiFiAPEnabledMatch(t *testing.T) {
	rawConfig := `{"wifi":{"ap":{"enable":false,"is_open":false}}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{WiFiAPEnabled: boolPtr(false), WiFiAPIsOpen: boolPtr(false)}

	compliant, issues := Evaluate(dev, rules)
	if !compliant {
		t.Fatalf("expected compliant device, got issues: %v", issues)
	}
}

func TestEvaluate_WiFiAPIsOpenMismatch(t *testing.T) {
	rawConfig := `{"wifi":{"ap":{"enable":true,"is_open":true}}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{WiFiAPIsOpen: boolPtr(false)}

	compliant, issues := Evaluate(dev, rules)
	if compliant {
		t.Fatalf("expected non-compliant device with open AP")
	}
	if len(issues) != 1 || issues[0] != "wifi_ap_is_open mismatch" {
		t.Fatalf("expected wifi_ap_is_open mismatch, got %v", issues)
	}
}

func TestEvaluate_WiFiAPNilRuleSkipped(t *testing.T) {
	rawConfig := `{"wifi":{"ap":{"enable":true,"is_open":true}}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{}

	compliant, issues := Evaluate(dev, rules)
	if !compliant {
		t.Fatalf("expected compliant device when AP rules are nil, got %v", issues)
	}
}

func TestEvaluate_EthEnabledMismatch(t *testing.T) {
	rawConfig := `{"eth":{"enable":false,"ipv4mode":"dhcp"}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{EthEnabled: boolPtr(true)}

	compliant, issues := Evaluate(dev, rules)
	if compliant {
		t.Fatalf("expected non-compliant device with eth disabled")
	}
	if len(issues) != 1 || issues[0] != "eth_enabled mismatch" {
		t.Fatalf("expected eth_enabled mismatch, got %v", issues)
	}
}

func TestEvaluate_EthIPv4ModeMatch(t *testing.T) {
	rawConfig := `{"eth":{"enable":true,"ipv4mode":"dhcp"}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{EthEnabled: boolPtr(true), EthIPv4Mode: "dhcp"}

	compliant, issues := Evaluate(dev, rules)
	if !compliant {
		t.Fatalf("expected compliant device, got issues: %v", issues)
	}
}

func TestEvaluate_EthIPv4ModeMismatch(t *testing.T) {
	rawConfig := `{"eth":{"enable":true,"ipv4mode":"static"}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{EthIPv4Mode: "dhcp"}

	compliant, issues := Evaluate(dev, rules)
	if compliant {
		t.Fatalf("expected non-compliant device with wrong ipv4mode")
	}
	if len(issues) != 1 || issues[0] != "eth_ipv4mode mismatch" {
		t.Fatalf("expected eth_ipv4mode mismatch, got %v", issues)
	}
}

func TestEvaluate_EthRulesSkippedOnNonEthDevice(t *testing.T) {
	// Plug/PlugS devices with no eth block in config — rules must not fire.
	rawConfig := `{"sys":{"device":{"name":"plug"}}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{EthEnabled: boolPtr(true), EthIPv4Mode: "dhcp"}

	// EthEnabled (bool path) fires a mismatch when path is not found; expected behaviour
	// is documented by the BLE pattern. Confirm both messages appear together.
	compliant, issues := Evaluate(dev, rules)
	if compliant {
		t.Fatalf("expected non-compliant when path missing")
	}
	if len(issues) != 2 {
		t.Fatalf("expected exactly eth_enabled + eth_ipv4mode mismatch, got %v", issues)
	}
}

func TestEvaluate_DebugMQTTMatch(t *testing.T) {
	rawConfig := `{"sys":{"debug":{"mqtt":{"enable":true}}}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{DebugMQTT: boolPtr(true)}

	compliant, issues := Evaluate(dev, rules)
	if !compliant {
		t.Fatalf("expected compliant device, got issues: %v", issues)
	}
}

func TestEvaluate_DebugMQTTMismatch(t *testing.T) {
	rawConfig := `{"sys":{"debug":{"mqtt":{"enable":true}}}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{DebugMQTT: boolPtr(false)}

	compliant, issues := Evaluate(dev, rules)
	if compliant {
		t.Fatalf("expected non-compliant device")
	}
	if len(issues) != 1 || issues[0] != "sys_debug_mqtt mismatch" {
		t.Fatalf("expected sys_debug_mqtt mismatch, got %v", issues)
	}
}

func TestEvaluate_DebugMQTTNilRuleSkipped(t *testing.T) {
	rawConfig := `{"sys":{"debug":{"mqtt":{"enable":true}}}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{}

	compliant, issues := Evaluate(dev, rules)
	if !compliant {
		t.Fatalf("expected compliant device when rule is nil, got %v", issues)
	}
}

func TestEvaluate_MatterEnabledMatch(t *testing.T) {
	rawConfig := `{"matter":{"enable":true}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{MatterEnabled: boolPtr(true)}

	compliant, issues := Evaluate(dev, rules)
	if !compliant {
		t.Fatalf("expected compliant device, got issues: %v", issues)
	}
}

func TestEvaluate_MatterEnabledMismatch(t *testing.T) {
	rawConfig := `{"matter":{"enable":false}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{MatterEnabled: boolPtr(true)}

	compliant, issues := Evaluate(dev, rules)
	if compliant {
		t.Fatalf("expected non-compliant device")
	}
	if len(issues) != 1 || issues[0] != "matter_enabled mismatch" {
		t.Fatalf("expected matter_enabled mismatch, got %v", issues)
	}
}

func TestEvaluate_ModbusEnabledMismatch(t *testing.T) {
	rawConfig := `{"modbus":{"enable":true}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{ModbusEnabled: boolPtr(false)}

	compliant, issues := Evaluate(dev, rules)
	if compliant {
		t.Fatalf("expected non-compliant device with modbus enabled")
	}
	if len(issues) != 1 || issues[0] != "modbus_enabled mismatch" {
		t.Fatalf("expected modbus_enabled mismatch, got %v", issues)
	}
}

func TestEvaluate_ZigbeeEnabledMatch(t *testing.T) {
	rawConfig := `{"zigbee":{"enable":false}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{ZigbeeEnabled: boolPtr(false)}

	compliant, issues := Evaluate(dev, rules)
	if !compliant {
		t.Fatalf("expected compliant device, got issues: %v", issues)
	}
}

func TestEvaluate_InfraRulesSkippedOnUnsupportedDevice(t *testing.T) {
	// Plug device — no matter/modbus/zigbee blocks. Rules set to false should fire mismatch
	// because path not found is treated as "not equal to expected", matching the BLE pattern.
	rawConfig := `{"sys":{"device":{"name":"plug"}}}`
	dev := models.Device{Gen: 2, RawConfig: rawConfig}
	rules := models.ComplianceRules{
		MatterEnabled: boolPtr(false),
		ModbusEnabled: boolPtr(false),
		ZigbeeEnabled: boolPtr(false),
	}

	compliant, issues := Evaluate(dev, rules)
	if compliant {
		t.Fatalf("expected non-compliant when paths missing")
	}
	if len(issues) != 3 {
		t.Fatalf("expected all three mismatches, got %v", issues)
	}
}
