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
	deviceName := effectiveDeviceName(dev)

	compareString := func(rule, got, label string) {
		if !matchesRuleString(rule, got, deviceName) {
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
	compareConfigString := func(rule, path, label string) {
		if strings.TrimSpace(rule) == "" {
			return
		}
		config := unmarshalMap(dev.RawConfig)
		got, found := resolvePath(config, path)
		if !found || strings.TrimSpace(got) != strings.TrimSpace(rule) {
			issues = append(issues, fmt.Sprintf("%s mismatch", label))
		}
	}
	compareConfigBool := func(rule *bool, path, label string) {
		if rule == nil {
			return
		}
		config := unmarshalMap(dev.RawConfig)
		got, found := resolvePath(config, path)
		if !found || got != strconv.FormatBool(*rule) {
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
	config := unmarshalMap(dev.RawConfig)

	compareString(rules.WiFiSSID, dev.WiFiSSID, "wifi_ssid")
	compareBoolPtr(rules.MQTTEnabled, dev.MQTTEnabled, "mqtt_enabled")
	compareString(rules.MQTTServer, dev.MQTTServer, "mqtt_server")
	compareString(rules.MQTTClientID, dev.MQTTClientID, "mqtt_client_id")
	compareString(rules.MQTTTopicPrefix, dev.MQTTTopicPrefix, "mqtt_topic_prefix")
	checkMQTTFlag(&issues, rules.MQTTRPCNtf, dev.MQTTFlagsNA, "rpc_ntf")
	checkMQTTFlag(&issues, rules.MQTTStatusNtf, dev.MQTTFlagsNA, "status_ntf")
	checkMQTTFlag(&issues, rules.MQTTEnableRPC, dev.MQTTFlagsNA, "enable_rpc")
	checkMQTTFlag(&issues, rules.MQTTEnableCtrl, dev.MQTTFlagsNA, "enable_control")
	compareBoolPtr(rules.CloudEnabled, dev.CloudEnabled, "cloud_enabled")
	compareBool(rules.CloudConnected, dev.CloudConnected, "cloud_connected")
	compareBool(rules.MQTTConnected, dev.MQTTConnected, "mqtt_connected")
	compareBoolPtr(rules.WSEnabled, dev.WSEnabled, "ws_enabled")
	compareBool(rules.WSConnected, dev.WSConnected, "ws_connected")
	compareString(rules.WSServer, dev.WSServer, "ws_server")
	compareWSTLSSettings(&issues, config, rules)
	compareBoolPtr(rules.BLEGWEnabled, dev.BLEGWEnabled, "ble_gw_enabled")
	compareConfigBool(rules.BLERPCEnabled, "ble.rpc.enable", "ble_rpc_enable")
	compareConfigBool(rules.BLEObserver, "ble.observer.enable", "ble_observer_enable")
	compareString(rules.TZ, dev.TZ, "tz")
	compareString(rules.SNTPServer, dev.SNTPServer, "sntp_server")
	compareFloat(rules.Lat, dev.Lat, "lat")
	compareFloat(rules.Lon, dev.Lon, "lon")
	compareBoolPtr(rules.EcoMode, dev.EcoMode, "eco_mode")
	compareBoolPtr(rules.Discoverable, dev.Discoverable, "discoverable")
	compareConfigBool(rules.DebugWebSocket, "sys.debug.websocket.enable", "sys_debug_websocket")
	compareConfigString(rules.DebugUDPHost, "sys.debug.udp.addr", "sys_debug_udp_host")
	compareRPCUDPPort(&issues, config, rules.RPCUDPPort)
	compareConfigBool(rules.WiFiAPEnabled, "wifi.ap.enable", "wifi_ap_enabled")
	compareConfigBool(rules.WiFiAPIsOpen, "wifi.ap.is_open", "wifi_ap_is_open")
	compareConfigBool(rules.EthEnabled, "eth.enable", "eth_enabled")
	compareConfigString(rules.EthIPv4Mode, "eth.ipv4mode", "eth_ipv4mode")
	compareConfigBool(rules.DebugMQTT, "sys.debug.mqtt.enable", "sys_debug_mqtt")
	compareConfigBool(rules.MatterEnabled, "matter.enable", "matter_enabled")
	compareConfigBool(rules.ModbusEnabled, "modbus.enable", "modbus_enabled")
	compareConfigBool(rules.ZigbeeEnabled, "zigbee.enable", "zigbee_enabled")
	// Firmware 2.0.0-beta1 compliance fields. EnhancedSecurity / TLSCertValid
	// are skipped when the device hasn't reported the underlying state, so
	// mixed fleets (1.x + 2.0) don't get false-positive failures.
	compareBoolPtr(rules.EnhancedSecurity, dev.EnhancedSecurity, "enhanced_security")
	compareBoolPtr(rules.TLSCertValid, dev.TLSCertValid, "tls_cert_valid")
	compareString(rules.WiFiHostname, dev.WiFiHostname, "wifi_hostname")
	if want := strings.TrimSpace(rules.AutoUpdateStage); want != "" {
		got := strings.TrimSpace(dev.FWAutoUpdate)
		// Empty got = "never read" — skip until a check has populated it.
		if got != "" && got != want {
			issues = append(issues, "auto_update_stage mismatch")
		}
	}
	evaluateCustomRules(&issues, dev, rules.CustomRules, deviceName)

	return len(issues) == 0, issues
}

func evaluateCustomRules(issues *[]string, dev models.Device, rules []models.CustomRule, deviceName string) {
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
		expected := substituteTokens(rule.Value, deviceName)
		if !checkOp(op, found, value, expected) {
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
	case "ble_rpc_enabled":
		return anyToString(dev.BLERPCEnabled), dev.BLERPCEnabled != nil
	case "ble_observer_enabled":
		return anyToString(dev.BLEObserverEnabled), dev.BLEObserverEnabled != nil
	case "cloud_enabled":
		return anyToString(dev.CloudEnabled), dev.CloudEnabled != nil
	case "mqtt_connected":
		return anyToString(dev.MQTTConnected), true
	case "tz":
		return dev.TZ, true
	case "sntp_server":
		return dev.SNTPServer, true
	case "eco_mode":
		return anyToString(dev.EcoMode), dev.EcoMode != nil
	case "discoverable":
		return anyToString(dev.Discoverable), dev.Discoverable != nil
	case "lat":
		return anyToString(dev.Lat), dev.Lat != nil
	case "lon":
		return anyToString(dev.Lon), dev.Lon != nil
	case "scheme":
		return dev.Scheme, dev.Scheme != ""
	case "enhanced_security":
		return anyToString(dev.EnhancedSecurity), dev.EnhancedSecurity != nil
	case "tls_cert_valid":
		return anyToString(dev.TLSCertValid), dev.TLSCertValid != nil
	case "wifi_hostname":
		return dev.WiFiHostname, dev.WiFiHostname != ""
	case "wifi_channel":
		return strconv.Itoa(dev.WiFiChannel), dev.WiFiChannel > 0
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

func substituteTokens(value, deviceName string) string {
	return strings.ReplaceAll(value, "{device_name}", deviceName)
}

func matchesRuleString(rule, got, deviceName string) bool {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return true
	}
	got = strings.TrimSpace(got)
	expected := strings.TrimSpace(substituteTokens(rule, deviceName))
	if expected == got {
		return true
	}
	if !strings.Contains(rule, "{device_name}") {
		return false
	}
	pattern := "^" + regexp.QuoteMeta(rule) + "$"
	pattern = strings.ReplaceAll(pattern, regexp.QuoteMeta("{device_name}"), `.+`)
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(got)
}

func effectiveDeviceName(dev models.Device) string {
	if strings.TrimSpace(dev.Name) != "" {
		return dev.Name
	}
	if strings.TrimSpace(dev.Serial) != "" {
		return dev.Serial
	}
	return dev.MAC
}

func compareWSTLSSettings(issues *[]string, config map[string]any, rules models.ComplianceRules) {
	if strings.TrimSpace(rules.WSTLSMode) == "" && strings.TrimSpace(rules.WSSSLCa) == "" {
		return
	}
	server, _ := resolvePath(config, "ws.server")
	server = strings.TrimSpace(server)
	if !strings.HasPrefix(strings.ToLower(server), "wss://") {
		if strings.TrimSpace(rules.WSTLSMode) != "" {
			*issues = append(*issues, "ws_tls_mode ignored because ws_server is non-tls")
		}
		if strings.TrimSpace(rules.WSSSLCa) != "" {
			*issues = append(*issues, "ws_ssl_ca ignored because ws_server is non-tls")
		}
		return
	}
	sslCA, _ := resolvePath(config, "ws.ssl_ca")
	sslCA = strings.TrimSpace(sslCA)
	if mode := strings.TrimSpace(rules.WSTLSMode); mode != "" {
		var gotMode string
		switch sslCA {
		case "*":
			gotMode = "no_validation"
		case "":
			gotMode = "default"
		default:
			gotMode = "user"
		}
		if gotMode != mode {
			*issues = append(*issues, "ws_tls_mode mismatch")
		}
	}
	if expectedCA := strings.TrimSpace(rules.WSSSLCa); expectedCA != "" && sslCA != expectedCA {
		*issues = append(*issues, "ws_ssl_ca mismatch")
	}
}

func compareRPCUDPPort(issues *[]string, config map[string]any, rule *int) {
	if rule == nil {
		return
	}
	got, found := resolvePath(config, "sys.rpc_udp.listen_port")
	if !found {
		*issues = append(*issues, "sys_rpc_udp_port mismatch")
		return
	}
	if strings.TrimSpace(got) != strconv.Itoa(*rule) {
		*issues = append(*issues, "sys_rpc_udp_port mismatch")
	}
}
