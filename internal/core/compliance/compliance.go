package compliance

import (
	"fmt"
	"math"
	"strings"

	"shellyadmin/internal/models"
)

func Evaluate(dev models.Device, rules models.ComplianceRules) (bool, []string) {
	var issues []string
	skipMQTT := dev.Gen == 1 && dev.CloudConnected

	compareString := func(rule, got, label string) {
		if rule != "" && rule != got {
			issues = append(issues, fmt.Sprintf("%s mismatch", label))
		}
	}
	compareBoolPtr := func(rule, got *bool, label string) {
		if rule == nil || got == nil {
			return
		}
		if *rule != *got {
			issues = append(issues, fmt.Sprintf("%s mismatch", label))
		}
	}
	compareBool := func(rule *bool, got bool, label string) {
		if rule != nil && *rule != got {
			issues = append(issues, fmt.Sprintf("%s mismatch", label))
		}
	}
	compareFloat := func(rule, got *float64, label string) {
		if rule == nil || got == nil {
			return
		}
		if math.Abs(*rule-*got) > 0.01 {
			issues = append(issues, fmt.Sprintf("%s mismatch", label))
		}
	}

	compareString(rules.WiFiSSID, dev.WiFiSSID, "wifi_ssid")
	if !skipMQTT {
		compareBoolPtr(rules.MQTTEnabled, dev.MQTTEnabled, "mqtt_enabled")
		compareString(rules.MQTTServer, dev.MQTTServer, "mqtt_server")
		compareString(rules.MQTTClientID, dev.MQTTClientID, "mqtt_client_id")
		compareString(rules.MQTTTopicPrefix, dev.MQTTTopicPrefix, "mqtt_topic_prefix")
		checkMQTTFlag(&issues, rules.MQTTRPCNtf, dev.MQTTFlagsNA, "rpc_ntf")
		checkMQTTFlag(&issues, rules.MQTTStatusNtf, dev.MQTTFlagsNA, "status_ntf")
		checkMQTTFlag(&issues, rules.MQTTEnableRPC, dev.MQTTFlagsNA, "enable_rpc")
		checkMQTTFlag(&issues, rules.MQTTEnableCtrl, dev.MQTTFlagsNA, "enable_control")
	}
	compareBool(rules.CloudConnected, dev.CloudConnected, "cloud_connected")
	if dev.Gen >= 2 {
		compareBoolPtr(rules.WSEnabled, dev.WSEnabled, "ws_enabled")
		compareBool(rules.WSConnected, dev.WSConnected, "ws_connected")
		compareString(rules.WSServer, dev.WSServer, "ws_server")
		compareBoolPtr(rules.BLEGWEnabled, dev.BLEGWEnabled, "ble_gw_enabled")
	}
	compareString(rules.TZ, dev.TZ, "tz")
	compareString(rules.SNTPServer, dev.SNTPServer, "sntp_server")
	compareFloat(rules.Lat, dev.Lat, "lat")
	compareFloat(rules.Lon, dev.Lon, "lon")
	compareString(rules.TimeFormat, dev.TimeFormat, "time_format")
	compareBoolPtr(rules.EcoMode, dev.EcoMode, "eco_mode")
	compareBoolPtr(rules.Discoverable, dev.Discoverable, "discoverable")

	return len(issues) == 0, issues
}

func checkMQTTFlag(issues *[]string, rule *bool, flagsCSV, flagName string) {
	if rule == nil {
		return
	}
	if hasMQTTFlag(flagsCSV, flagName) != *rule {
		*issues = append(*issues, fmt.Sprintf("mqtt %s mismatch", flagName))
	}
}

func hasMQTTFlag(flagsCSV, flagName string) bool {
	for _, f := range strings.Split(flagsCSV, ",") {
		if strings.TrimSpace(f) == flagName {
			return true
		}
	}
	return false
}
