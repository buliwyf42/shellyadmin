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
		MQTTServer:      "mqtt.home.lan:1883",
		MQTTClientID:    "shelly-plugOD-01",
		MQTTTopicPrefix: "shelly/shelly-plugOD-01",
		CloudConnected:  true,
		WSConnected:     true,
		TZ:              "Europe/Berlin",
		TimeFormat:      "24h",
		WiFiSSID:        "buliwyf_iot",
		RawConfig:       string(rawConfig),
	}

	rules := models.ComplianceRules{
		WiFiSSID:        "buliwyf_iot",
		MQTTEnabled:     boolPtr(true),
		MQTTServer:      "mqtt.home.lan:1883",
		MQTTClientID:    "{device_name}",
		MQTTTopicPrefix: "shelly/{device_name}",
		CloudConnected:  boolPtr(true),
		WSConnected:     boolPtr(true),
		TZ:              "Europe/Berlin",
		TimeFormat:      "24h",
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
		MQTTServer:      "mqtt.home.lan:1883",
		MQTTClientID:    "shelly-plugOD-01",
		MQTTTopicPrefix: "shelly/shelly-plugOD-01",
		RawConfig:       string(rawConfig),
	}

	rules := models.ComplianceRules{
		MQTTEnabled:     boolPtr(true),
		MQTTServer:      "mqtt.home.lan:1883",
		MQTTClientID:    "{device_name}",
		MQTTTopicPrefix: "shelly/{device_name}",
	}

	compliant, issues := Evaluate(dev, rules)
	if !compliant {
		t.Fatalf("expected compliant device with template fallback, got issues: %v", issues)
	}
}

func TestEvaluate_OTAAutoUpdateMatch(t *testing.T) {
	rawConfig, _ := json.Marshal(map[string]any{
		"ota": map[string]any{
			"auto_update": "stable",
		},
	})
	dev := models.Device{RawConfig: string(rawConfig)}
	rules := models.ComplianceRules{OTAAutoUpdate: "stable"}

	compliant, issues := Evaluate(dev, rules)
	if !compliant {
		t.Fatalf("expected compliant device, got issues: %v", issues)
	}
}

func TestEvaluate_OTAAutoUpdateMismatch(t *testing.T) {
	rawConfig, _ := json.Marshal(map[string]any{
		"ota": map[string]any{
			"auto_update": "beta",
		},
	})
	dev := models.Device{RawConfig: string(rawConfig)}
	rules := models.ComplianceRules{OTAAutoUpdate: "stable"}

	compliant, issues := Evaluate(dev, rules)
	if compliant {
		t.Fatalf("expected non-compliant device")
	}
	if len(issues) != 1 || issues[0] != "ota_auto_update mismatch" {
		t.Fatalf("expected ota mismatch, got %v", issues)
	}
}

func TestEvaluate_OTAAutoUpdateUnsupported(t *testing.T) {
	dev := models.Device{RawConfig: `{"sys":{"cfg_rev":1}}`}
	rules := models.ComplianceRules{OTAAutoUpdate: "stable"}

	compliant, issues := Evaluate(dev, rules)
	if compliant {
		t.Fatalf("expected non-compliant device")
	}
	if len(issues) != 1 || issues[0] != "ota_auto_update unsupported" {
		t.Fatalf("expected ota unsupported, got %v", issues)
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

func TestEvaluate_TimeFormatSkippedOnGen2(t *testing.T) {
	// Gen2+ devices have no 12h/24h setting — the time_format rule is silently
	// skipped so it does not produce false-positive compliance failures.
	dev := models.Device{Gen: 4, RawConfig: `{"sys":{"cfg_rev":1}}`}
	rules := models.ComplianceRules{TimeFormat: "24h"}

	compliant, issues := Evaluate(dev, rules)
	if !compliant {
		t.Fatalf("expected compliant: time_format rule should be skipped on Gen2+, got issues: %v", issues)
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

func TestEvaluate_MQTTConnectedCheckOnGen2Only(t *testing.T) {
	// Gen1 device: mqtt_connected rule should be skipped.
	dev1 := models.Device{Gen: 1, MQTTConnected: false}
	rules := models.ComplianceRules{MQTTConnected: boolPtr(true)}
	compliant, issues := Evaluate(dev1, rules)
	if !compliant {
		t.Fatalf("Gen1: expected mqtt_connected rule to be skipped, got issues: %v", issues)
	}

	// Gen2+ device: mqtt_connected matches.
	dev2 := models.Device{Gen: 2, MQTTConnected: true}
	compliant, issues = Evaluate(dev2, rules)
	if !compliant {
		t.Fatalf("Gen2: expected compliant device, got issues: %v", issues)
	}

	// Gen2+ device: mismatch.
	dev3 := models.Device{Gen: 3, MQTTConnected: false}
	compliant, issues = Evaluate(dev3, rules)
	if compliant {
		t.Fatalf("Gen2: expected non-compliant device")
	}
	if len(issues) != 1 || issues[0] != "mqtt_connected mismatch" {
		t.Fatalf("expected mqtt_connected mismatch, got %v", issues)
	}
}
