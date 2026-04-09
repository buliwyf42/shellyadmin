package compliance

import (
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
