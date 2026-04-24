package provisioner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"shellyadmin/internal/util"
)

type DeviceInfo struct {
	Name  string `json:"name"`
	Model string `json:"model"`
	FW    string `json:"fw"`
	Gen   int    `json:"gen"`
	IP    string `json:"ip"`
}

type SectionResult struct {
	Section         string `json:"section"`
	Status          string `json:"status"`
	Detail          string `json:"detail"`
	RestartRequired bool   `json:"restart_required,omitempty"`
}

func ProvisionDevice(ctx context.Context, ip string, template map[string]interface{}, timeout time.Duration) (DeviceInfo, []SectionResult) {
	client := &http.Client{Timeout: timeout}
	info, serial := identify(ctx, client, ip)
	name := resolvedDeviceName(ctx, client, ip, info, serial)
	if strings.TrimSpace(info.Name) == "" {
		info.Name = name
	}
	applied := substitute(template, name).(map[string]interface{})
	results := make([]SectionResult, 0, len(applied))
	for section, raw := range applied {
		result := applySection(ctx, client, ip, info.Gen, serial, section, raw)
		results = append(results, result)
	}
	return info, results
}

func identify(ctx context.Context, client *http.Client, ip string) (DeviceInfo, string) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+ip+"/shelly", nil)
	resp, err := client.Do(req)
	if err != nil {
		return DeviceInfo{IP: ip}, ""
	}
	defer resp.Body.Close()
	var base struct {
		Name  string `json:"name"`
		Model string `json:"model"`
		FW    string `json:"fw"`
		Gen   int    `json:"gen"`
		ID    string `json:"id"`
		MAC   string `json:"mac"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&base)
	return DeviceInfo{Name: base.Name, Model: base.Model, FW: base.FW, Gen: base.Gen, IP: ip}, util.FirstNonEmpty(base.ID, base.MAC)
}

func applySection(ctx context.Context, client *http.Client, ip string, gen int, serial, section string, raw interface{}) SectionResult {
	payload, _ := raw.(map[string]interface{})
	switch strings.ToLower(section) {
	case "gen2_rpc":
		for method, item := range payload {
			methodPayload, ok := item.(map[string]interface{})
			if !ok {
				return SectionResult{Section: section, Status: "failed", Detail: "method payload must be object"}
			}
			result := rpcSection(ctx, client, ip, method, methodPayload, section)
			if result.Status != "ok" {
				return result
			}
		}
		return SectionResult{Section: section, Status: "ok", Detail: "gen2 methods applied"}
	case "gen1_http":
		return SectionResult{Section: section, Status: "skipped", Detail: "gen1 not supported"}
	case "mqtt":
		return rpcConfigSection(ctx, client, ip, "MQTT.SetConfig", payload, section)
	case "sys":
		config, warning := normalizeSysPayload(payload)
		return applyConfigWithWarning(ctx, client, ip, "Sys.SetConfig", config, section, warning)
	case "ws":
		config, warning, err := normalizeWSPayload(payload)
		if err != nil {
			return SectionResult{Section: section, Status: "failed", Detail: err.Error()}
		}
		return applyConfigWithWarning(ctx, client, ip, "WS.SetConfig", config, section, warning)
	case "ble":
		return rpcConfigSection(ctx, client, ip, "BLE.SetConfig", payload, section)
	case "matter":
		return rpcConfigSection(ctx, client, ip, "Matter.SetConfig", payload, section)
	case "cloud":
		return rpcConfigSection(ctx, client, ip, "Cloud.SetConfig", payload, section)
	case "wifi":
		return rpcConfigSection(ctx, client, ip, "Wifi.SetConfig", payload, section)
	case "eth":
		return rpcConfigSection(ctx, client, ip, "Eth.SetConfig", payload, section)
	case "kvs":
		for key, val := range payload {
			result := rpcSection(ctx, client, ip, "KVS.Set", map[string]interface{}{"key": key, "value": val}, section)
			if result.Status != "ok" {
				return result
			}
		}
		return SectionResult{Section: section, Status: "ok", Detail: "keys written"}
	case "script":
		for idStr, val := range payload {
			id, err := strconv.Atoi(idStr)
			if err != nil {
				return SectionResult{Section: section, Status: "failed", Detail: fmt.Sprintf("script id %q is not an integer", idStr)}
			}
			config, ok := val.(map[string]interface{})
			if !ok {
				return SectionResult{Section: section, Status: "failed", Detail: fmt.Sprintf("script %s config must be an object", idStr)}
			}
			result := rpcSection(ctx, client, ip, "Script.SetConfig", map[string]interface{}{"id": id, "config": config}, section)
			if result.Status != "ok" {
				return result
			}
		}
		return SectionResult{Section: section, Status: "ok", Detail: "scripts configured"}
	case "auth":
		pass := fmt.Sprint(payload["pass"])
		ha1Input := "admin:" + serial + ":" + pass
		sum := sha256.Sum256([]byte(ha1Input))
		authPayload := map[string]interface{}{
			"user":  "admin",
			"realm": serial,
			"ha1":   hex.EncodeToString(sum[:]),
		}
		return rpcSection(ctx, client, ip, "Shelly.SetAuth", authPayload, section)
	default:
		method := strings.ToUpper(section[:1]) + section[1:] + ".SetConfig"
		return rpcConfigSection(ctx, client, ip, method, payload, section)
	}
}

func resolvedDeviceName(ctx context.Context, client *http.Client, ip string, info DeviceInfo, serial string) string {
	if name := configuredDeviceName(ctx, client, ip); name != "" {
		return name
	}
	return util.FirstNonEmpty(info.Name, serial, ip)
}

func configuredDeviceName(ctx context.Context, client *http.Client, ip string) string {
	reqBody := map[string]any{
		"id":     1,
		"method": "Shelly.GetConfig",
	}
	buf, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+ip+"/rpc", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return ""
	}
	var payload struct {
		Result map[string]any `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ""
	}
	sys, _ := payload.Result["sys"].(map[string]any)
	device, _ := sys["device"].(map[string]any)
	return strings.TrimSpace(anyString(device["name"]))
}

func applyConfigWithWarning(ctx context.Context, client *http.Client, ip, method string, payload map[string]interface{}, section, warning string) SectionResult {
	if len(payload) == 0 {
		if strings.TrimSpace(warning) == "" {
			return SectionResult{Section: section, Status: "skipped", Detail: "no supported fields to apply"}
		}
		return SectionResult{Section: section, Status: "skipped", Detail: warning}
	}
	result := rpcConfigSection(ctx, client, ip, method, payload, section)
	if result.Status == "ok" && strings.TrimSpace(warning) != "" {
		result.Detail = result.Detail + "; " + warning
	}
	return result
}

func normalizeSysPayload(payload map[string]interface{}) (map[string]interface{}, string) {
	if payload == nil {
		return nil, ""
	}
	out := map[string]interface{}{}
	deviceCfg := map[string]interface{}{}
	location := map[string]interface{}{}
	sntp := map[string]interface{}{}
	debugCfg := map[string]interface{}{}
	debugWS := map[string]interface{}{}
	debugUDP := map[string]interface{}{}
	debugMQTT := map[string]interface{}{}
	rpcUDP := map[string]interface{}{}
	var warnings []string

	if device, ok := payload["device"].(map[string]interface{}); ok {
		copyKnownKeys(deviceCfg, device, "name", "eco_mode", "discoverable")
	}
	if name := strings.TrimSpace(anyString(payload["name"])); name != "" && deviceCfg["name"] == nil {
		deviceCfg["name"] = name
	}

	if nestedLocation, ok := payload["location"].(map[string]interface{}); ok {
		copyKnownKeys(location, nestedLocation, "tz", "lat", "lon")
	}
	if tz := strings.TrimSpace(anyString(payload["tz"])); tz != "" && location["tz"] == nil {
		location["tz"] = tz
	}
	if lat, ok := numericValue(payload["lat"]); ok && location["lat"] == nil {
		location["lat"] = lat
	}
	if lon, ok := numericValue(payload["lon"]); ok && location["lon"] == nil {
		location["lon"] = lon
	}
	if lon, ok := numericValue(payload["lng"]); ok && location["lon"] == nil {
		location["lon"] = lon
	}

	if nestedSNTP, ok := payload["sntp"].(map[string]interface{}); ok {
		copyKnownKeys(sntp, nestedSNTP, "server")
	}

	if debug, ok := payload["debug"].(map[string]interface{}); ok {
		if ws, ok := debug["websocket"].(map[string]interface{}); ok {
			copyKnownKeys(debugWS, ws, "enable")
		}
		if udp, ok := debug["udp"].(map[string]interface{}); ok {
			copyKnownKeys(debugUDP, udp, "addr")
		}
		if mqtt, ok := debug["mqtt"].(map[string]interface{}); ok {
			copyKnownKeys(debugMQTT, mqtt, "enable")
		}
	}
	if legacyDebug, ok := payload["dbg"].(map[string]interface{}); ok {
		if enabled, ok := legacyDebug["websocket_enable"]; ok && debugWS["enable"] == nil {
			debugWS["enable"] = enabled
		}
		if addr := strings.TrimSpace(anyString(legacyDebug["udp_addr"])); addr != "" && debugUDP["addr"] == nil {
			debugUDP["addr"] = addr
		}
	}
	if len(debugWS) > 0 {
		debugCfg["websocket"] = debugWS
	}
	if len(debugUDP) > 0 {
		debugCfg["udp"] = debugUDP
	}
	if len(debugMQTT) > 0 {
		debugCfg["mqtt"] = debugMQTT
	}

	if nestedRPCUDP, ok := payload["rpc_udp"].(map[string]interface{}); ok {
		if port, ok := numericValue(nestedRPCUDP["listen_port"]); ok {
			rpcUDP["listen_port"] = port
		} else if port, ok := numericValue(nestedRPCUDP["port"]); ok {
			rpcUDP["listen_port"] = port
		}
	}

	if _, exists := payload["clock_mode"]; exists {
		warnings = append(warnings, "sys.clock_mode unsupported on this device")
	}

	copyKnownKeys(out, payload, "profile", "addon_type")

	if len(deviceCfg) > 0 {
		out["device"] = deviceCfg
	}
	if len(location) > 0 {
		out["location"] = location
	}
	if len(sntp) > 0 {
		out["sntp"] = sntp
	}
	if len(debugCfg) > 0 {
		out["debug"] = debugCfg
	}
	if len(rpcUDP) > 0 {
		out["rpc_udp"] = rpcUDP
	}
	return out, strings.Join(warnings, "; ")
}

func normalizeWSPayload(payload map[string]interface{}) (map[string]interface{}, string, error) {
	if payload == nil {
		return nil, "", nil
	}
	out := map[string]interface{}{}
	var warnings []string

	if enabled, ok := payload["enable"]; ok {
		out["enable"] = enabled
	}
	server := strings.TrimSpace(anyString(payload["server"]))
	if server != "" {
		out["server"] = server
	}
	tlsMode := strings.TrimSpace(anyString(payload["tls_mode"]))
	sslCA := strings.TrimSpace(anyString(payload["ssl_ca"]))

	if isTLSServerURL(server) {
		switch tlsMode {
		case "", "default":
			// Device default TLS validation: omit ssl_ca.
		case "no_validation":
			out["ssl_ca"] = "*"
		case "user":
			if sslCA == "" {
				return nil, "", fmt.Errorf("ws.ssl_ca is required when ws.tls_mode is user")
			}
			out["ssl_ca"] = sslCA
		default:
			return nil, "", fmt.Errorf("unsupported ws.tls_mode %q", tlsMode)
		}
	} else {
		if tlsMode != "" || sslCA != "" {
			warnings = append(warnings, "ws TLS mode ignored because ws.server is non-TLS")
		}
	}
	return out, strings.Join(warnings, "; "), nil
}

func copyKnownKeys(dst, src map[string]interface{}, keys ...string) {
	for _, key := range keys {
		if value, ok := src[key]; ok {
			dst[key] = value
		}
	}
}

func numericValue(raw interface{}) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, false
		}
		return value, true
	case float32:
		f := float64(value)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case json.Number:
		f, err := value.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func isTLSServerURL(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	if parsed, err := url.Parse(raw); err == nil {
		return strings.EqualFold(parsed.Scheme, "wss")
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "wss://")
}

func rpcConfigSection(ctx context.Context, client *http.Client, ip, method string, payload map[string]interface{}, section string) SectionResult {
	return rpcSection(ctx, client, ip, method, map[string]interface{}{"config": payload}, section)
}

func rpcSection(ctx context.Context, client *http.Client, ip, method string, payload map[string]interface{}, section string) SectionResult {
	reqBody := map[string]any{
		"id":     1,
		"method": method,
		"params": payload,
	}
	buf, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+ip+"/rpc", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return SectionResult{Section: section, Status: "failed", Detail: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 404 {
		// Shelly devices return HTTP 404 when a method/handler is not available on
		// the specific model. Treat this the same as a JSON-RPC -32601 error.
		return SectionResult{Section: section, Status: "skipped", Detail: "method not supported by this device"}
	}
	if resp.StatusCode >= 400 {
		return SectionResult{Section: section, Status: "failed", Detail: util.FirstNonEmpty(rpcErrorDetail(body), resp.Status)}
	}

	var rpcResp struct {
		Error  any `json:"error"`
		Result struct {
			RestartRequired bool `json:"restart_required"`
		} `json:"result"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &rpcResp); err == nil && rpcResp.Error != nil {
			if isMethodNotFound(rpcResp.Error) {
				return SectionResult{Section: section, Status: "skipped", Detail: "method not supported by this device"}
			}
			return SectionResult{Section: section, Status: "failed", Detail: rpcErrorValue(rpcResp.Error)}
		}
	}
	return SectionResult{Section: section, Status: "ok", Detail: method, RestartRequired: rpcResp.Result.RestartRequired}
}

func isMethodNotFound(raw any) bool {
	obj, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	code, ok := obj["code"]
	if !ok {
		return false
	}
	switch v := code.(type) {
	case float64:
		return int(v) == -32601 || int(v) == 404
	case int:
		return v == -32601 || v == 404
	case json.Number:
		n, err := v.Int64()
		return err == nil && (n == -32601 || n == 404)
	}
	return false
}

func rpcErrorDetail(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return strings.TrimSpace(string(body))
	}
	if raw, ok := payload["error"]; ok {
		return rpcErrorValue(raw)
	}
	return strings.TrimSpace(string(body))
}

func rpcErrorValue(raw any) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]any:
		msg := util.FirstNonEmpty(anyString(value["message"]), anyString(value["msg"]), anyString(value["error"]))
		code := anyString(value["code"])
		if msg != "" && code != "" {
			return fmt.Sprintf("%s (%s)", msg, code)
		}
		if msg != "" {
			return msg
		}
		encoded, _ := json.Marshal(value)
		return string(encoded)
	default:
		encoded, _ := json.Marshal(value)
		return string(encoded)
	}
}

func anyString(raw any) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
	default:
		if value == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func substitute(v interface{}, name string) interface{} {
	switch val := v.(type) {
	case string:
		return strings.ReplaceAll(val, "{device_name}", name)
	case map[string]interface{}:
		out := map[string]interface{}{}
		for k, v2 := range val {
			out[k] = substitute(v2, name)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, v2 := range val {
			out[i] = substitute(v2, name)
		}
		return out
	default:
		return v
	}
}
