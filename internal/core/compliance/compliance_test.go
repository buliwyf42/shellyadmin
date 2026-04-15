package compliance

import (
	"encoding/json"
	"testing"

	"shellyadmin/internal/models"
)

func boolPtr(v bool) *bool { return &v }

func TestEvaluate_DeviceNameTokenSubstitutionForMQTT(t *testing.T) {
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
	dev := models.Device{
		Gen:             3,
		Name:            "different-display-name",
		MQTTEnabled:     boolPtr(true),
		MQTTServer:      "mqtt.home.lan:1883",
		MQTTClientID:    "shelly-plugOD-01",
		MQTTTopicPrefix: "shelly/shelly-plugOD-01",
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
