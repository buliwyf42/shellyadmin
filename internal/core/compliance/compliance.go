package compliance

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
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
	if !(dev.Gen <= 1 && strings.TrimSpace(dev.TZ) == "") {
		compareString(rules.TZ, dev.TZ, "tz")
	}
	compareString(rules.SNTPServer, dev.SNTPServer, "sntp_server")
	compareFloat(rules.Lat, dev.Lat, "lat")
	compareFloat(rules.Lon, dev.Lon, "lon")
	compareString(rules.TimeFormat, dev.TimeFormat, "time_format")
	compareBoolPtr(rules.EcoMode, dev.EcoMode, "eco_mode")
	compareBoolPtr(rules.Discoverable, dev.Discoverable, "discoverable")
	evaluateCustomRules(&issues, dev, rules.CustomRules)

	return len(issues) == 0, issues
}

func evaluateCustomRules(issues *[]string, dev models.Device, rules []models.CustomRule) {
	if len(rules) == 0 {
		return
	}
	config := unmarshalMap(dev.RawConfig)
	status := unmarshalMap(dev.RawStatus)
	for _, rule := range rules {
		if rule.Path == "" {
			continue
		}
		if rule.GenMin > 0 && dev.Gen < rule.GenMin {
			continue
		}
		if rule.GenMax > 0 && dev.Gen > rule.GenMax {
			continue
		}
		source := strings.ToLower(strings.TrimSpace(rule.Source))
		if source == "" {
			source = "device"
		}
		op := strings.ToLower(strings.TrimSpace(rule.Op))
		if op == "" {
			op = "eq"
		}
		value, found := lookupValue(source, dev, config, status, rule.Path)
		if !checkOp(op, found, value, rule.Value) {
			label := strings.TrimSpace(rule.Label)
			if label == "" {
				label = rule.Path
			}
			*issues = append(*issues, fmt.Sprintf("%s mismatch", label))
		}
	}
}

func lookupValue(source string, dev models.Device, config, status map[string]any, path string) (string, bool) {
	switch source {
	case "config":
		return resolvePath(config, path)
	case "status":
		return resolvePath(status, path)
	default:
		return resolveDevicePath(dev, path)
	}
}

func resolvePath(root map[string]any, path string) (string, bool) {
	if len(root) == 0 || path == "" {
		return "", false
	}
	cur := any(root)
	for _, part := range strings.Split(path, ".") {
		key := strings.TrimSpace(part)
		if key == "" {
			return "", false
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		next, ok := obj[key]
		if !ok {
			return "", false
		}
		cur = next
	}
	return anyToString(cur), true
}

func resolveDevicePath(dev models.Device, path string) (string, bool) {
	switch path {
	case "wifi_ssid":
		return dev.WiFiSSID, true
	case "mqtt_enabled":
		return anyToString(dev.MQTTEnabled), dev.MQTTEnabled != nil
	case "mqtt_server":
		return dev.MQTTServer, true
	case "mqtt_client_id":
		return dev.MQTTClientID, true
	case "mqtt_topic_prefix":
		return dev.MQTTTopicPrefix, true
	case "cloud_connected":
		return anyToString(dev.CloudConnected), true
	case "ws_enabled":
		return anyToString(dev.WSEnabled), dev.WSEnabled != nil
	case "ws_connected":
		return anyToString(dev.WSConnected), true
	case "ws_server":
		return dev.WSServer, true
	case "ble_gw_enabled":
		return anyToString(dev.BLEGWEnabled), dev.BLEGWEnabled != nil
	case "tz":
		return dev.TZ, true
	case "sntp_server":
		return dev.SNTPServer, true
	case "time_format":
		return dev.TimeFormat, true
	case "eco_mode":
		return anyToString(dev.EcoMode), dev.EcoMode != nil
	case "discoverable":
		return anyToString(dev.Discoverable), dev.Discoverable != nil
	case "lat":
		return anyToString(dev.Lat), dev.Lat != nil
	case "lon":
		return anyToString(dev.Lon), dev.Lon != nil
	default:
		return "", false
	}
}

func checkOp(op string, found bool, got, expected string) bool {
	switch op {
	case "exists":
		return found
	case "ne":
		return !found || got != expected
	case "contains":
		return found && strings.Contains(strings.ToLower(got), strings.ToLower(expected))
	case "regex":
		if !found {
			return false
		}
		pattern, err := regexp.Compile(expected)
		if err != nil {
			return false
		}
		return pattern.MatchString(got)
	default:
		return found && got == expected
	}
}

func unmarshalMap(raw string) map[string]any {
	if raw == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

func anyToString(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case bool:
		return strconv.FormatBool(value)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(value), 'f', -1, 64)
	case int:
		return strconv.Itoa(value)
	case *bool:
		if value == nil {
			return ""
		}
		return strconv.FormatBool(*value)
	case *float64:
		if value == nil {
			return ""
		}
		return strconv.FormatFloat(*value, 'f', -1, 64)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
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
