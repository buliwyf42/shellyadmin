package provisioner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"shellyadmin/internal/core/firmware"
	"shellyadmin/internal/core/shellyclient"
	"shellyadmin/internal/util"
)

// Options carries the per-device configuration used to build a shellyclient.
// Empty values yield an unauthenticated http client (legacy behaviour).
type Options struct {
	Timeout       time.Duration
	Scheme        string
	Username      string
	Password      string
	HA1           string
	AllowInsecure bool
}

func (o Options) toClientOptions() shellyclient.Options {
	out := shellyclient.Options{
		Timeout:  o.Timeout,
		Scheme:   o.Scheme,
		Username: o.Username,
		Password: o.Password,
		HA1:      o.HA1,
	}
	if o.AllowInsecure {
		out.TLSPolicy = shellyclient.TLSSkip
	}
	return out
}

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

// ProvisionDevice keeps the original timeout-only signature and is used by
// callers that don't carry credentials (or run against unauthenticated devices).
func ProvisionDevice(ctx context.Context, ip string, template map[string]interface{}, timeout time.Duration) (DeviceInfo, []SectionResult) {
	return ProvisionDeviceWithOptions(ctx, ip, template, Options{Timeout: timeout})
}

// ProvisionDeviceWithOptions threads digest auth + scheme into every RPC.
func ProvisionDeviceWithOptions(ctx context.Context, ip string, template map[string]interface{}, opts Options) (DeviceInfo, []SectionResult) {
	client := shellyclient.New(opts.toClientOptions())
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

func identify(ctx context.Context, client *shellyclient.Client, ip string) (DeviceInfo, string) {
	out, err := client.Probe(ctx, ip)
	if err != nil {
		return DeviceInfo{IP: ip}, ""
	}
	info := DeviceInfo{
		IP:    ip,
		Name:  stringField(out, "name"),
		Model: stringField(out, "model"),
		FW:    stringField(out, "fw"),
	}
	if g, ok := out["gen"].(float64); ok {
		info.Gen = int(g)
	}
	serial := util.FirstNonEmpty(stringField(out, "id"), stringField(out, "mac"))
	return info, serial
}

func applySection(ctx context.Context, client *shellyclient.Client, ip string, gen int, serial, section string, raw interface{}) SectionResult {
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
		// FW 2.0.0-beta1 removed the global ble.enable flag; pass-through callers
		// that still set it would otherwise hit the device's stricter validator.
		config, warning := normalizeBLEPayload(payload)
		return applyConfigWithWarning(ctx, client, ip, "BLE.SetConfig", config, section, warning)
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
	case "cover":
		// Explicit cover handler enables compliance/normalization hooks; the
		// underlying RPC is the same Cover.SetConfig the catch-all would route to.
		config, warning := normalizeCoverPayload(payload)
		return applyConfigWithWarning(ctx, client, ip, "Cover.SetConfig", config, section, warning)
	case "lnm":
		// FW 2.0.0-beta1 Local Network Messaging. Method name is LNM.SetConfig
		// (all-caps), which the catch-all's title-case mapping wouldn't produce.
		return rpcConfigSection(ctx, client, ip, "LNM.SetConfig", payload, section)
	case "auto_update":
		// Synthesizes a Schedule.* job rather than calling a SetConfig method —
		// see internal/core/firmware/autoupdate.go for why.
		mode := ""
		switch v := raw.(type) {
		case string:
			mode = v
		case map[string]interface{}:
			if s, ok := v["stage"].(string); ok {
				mode = s
			} else if s, ok := v["mode"].(string); ok {
				mode = s
			} else {
				return SectionResult{Section: section, Status: "failed", Detail: "auto_update payload must include stage"}
			}
		default:
			return SectionResult{Section: section, Status: "failed", Detail: "auto_update payload must be \"off|stable|beta\" or {stage:...}"}
		}
		if err := firmware.SetAutoUpdateOnClient(ctx, client, ip, mode); err != nil {
			return SectionResult{Section: section, Status: "failed", Detail: err.Error()}
		}
		return SectionResult{Section: section, Status: "ok", Detail: "auto-update set to " + strings.ToLower(strings.TrimSpace(mode))}
	case "webhooks":
		// FW 2.0.0-beta1 webhook configuration. Webhooks aren't a typical
		// SetConfig surface — they're managed via Webhook.Create/Update/Delete
		// and Webhook.DeleteAll. We accept a small ops-style payload that drives
		// the underlying RPCs in a deterministic order: delete_all → delete →
		// update → create.
		return applyWebhooksSection(ctx, client, ip, payload, section)
	default:
		method := strings.ToUpper(section[:1]) + section[1:] + ".SetConfig"
		return rpcConfigSection(ctx, client, ip, method, payload, section)
	}
}

// applyWebhooksSection drives Webhook.* RPCs from a single template section.
// Accepted shape (any subset of these keys):
//
//	{
//	  "delete_all": true,
//	  "delete":     [<id>, <id>, ...],
//	  "update":     [{ "id": <id>, ... }, ...],
//	  "create":     [{ "cid": <comp-id>, "event": "...", "urls": [...] }, ...]
//	}
//
// Ops apply in delete_all → delete → update → create order so a template can
// declaratively wipe-and-replace the device's webhook set.
func applyWebhooksSection(ctx context.Context, client *shellyclient.Client, ip string, payload map[string]interface{}, section string) SectionResult {
	if payload == nil {
		return SectionResult{Section: section, Status: "skipped", Detail: "empty webhook payload"}
	}
	count := 0

	if del, ok := payload["delete_all"].(bool); ok && del {
		if _, err := client.RPC(ctx, ip, "Webhook.DeleteAll", nil); err != nil {
			return webhookFailure(section, "delete_all", err)
		}
		count++
	}

	if raw, ok := payload["delete"].([]interface{}); ok {
		for _, idRaw := range raw {
			id, ok := numericValue(idRaw)
			if !ok {
				return SectionResult{Section: section, Status: "failed", Detail: fmt.Sprintf("delete: id %v is not numeric", idRaw)}
			}
			if _, err := client.RPC(ctx, ip, "Webhook.Delete", map[string]any{"id": int(id)}); err != nil {
				return webhookFailure(section, fmt.Sprintf("delete id=%d", int(id)), err)
			}
			count++
		}
	}

	if raw, ok := payload["update"].([]interface{}); ok {
		for _, item := range raw {
			obj, ok := item.(map[string]interface{})
			if !ok {
				return SectionResult{Section: section, Status: "failed", Detail: "update entries must be objects"}
			}
			if _, err := client.RPC(ctx, ip, "Webhook.Update", obj); err != nil {
				return webhookFailure(section, fmt.Sprintf("update %v", obj["id"]), err)
			}
			count++
		}
	}

	if raw, ok := payload["create"].([]interface{}); ok {
		for _, item := range raw {
			obj, ok := item.(map[string]interface{})
			if !ok {
				return SectionResult{Section: section, Status: "failed", Detail: "create entries must be objects"}
			}
			if _, err := client.RPC(ctx, ip, "Webhook.Create", obj); err != nil {
				return webhookFailure(section, fmt.Sprintf("create event=%v", obj["event"]), err)
			}
			count++
		}
	}

	if count == 0 {
		return SectionResult{Section: section, Status: "skipped", Detail: "no webhook operations specified"}
	}
	return SectionResult{Section: section, Status: "ok", Detail: fmt.Sprintf("applied %d webhook operations", count)}
}

func webhookFailure(section, opLabel string, err error) SectionResult {
	if shellyclient.IsMethodNotFound(err) {
		return SectionResult{Section: section, Status: "skipped", Detail: opLabel + ": webhook RPC not supported on this device"}
	}
	if errors.Is(err, shellyclient.ErrAuthRequired) {
		return SectionResult{Section: section, Status: "failed", Detail: opLabel + ": authentication required"}
	}
	if errors.Is(err, shellyclient.ErrAuthLockout) {
		return SectionResult{Section: section, Status: "failed", Detail: opLabel + ": device locked (brute-force protection)"}
	}
	return SectionResult{Section: section, Status: "failed", Detail: opLabel + ": " + err.Error()}
}

func resolvedDeviceName(ctx context.Context, client *shellyclient.Client, ip string, info DeviceInfo, serial string) string {
	if name := configuredDeviceName(ctx, client, ip); name != "" {
		return name
	}
	return util.FirstNonEmpty(info.Name, serial, ip)
}

func configuredDeviceName(ctx context.Context, client *shellyclient.Client, ip string) string {
	result, err := client.RPC(ctx, ip, "Shelly.GetConfig", nil)
	if err != nil {
		return ""
	}
	sys, _ := result["sys"].(map[string]any)
	device, _ := sys["device"].(map[string]any)
	return strings.TrimSpace(anyString(device["name"]))
}

func applyConfigWithWarning(ctx context.Context, client *shellyclient.Client, ip, method string, payload map[string]interface{}, section, warning string) SectionResult {
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

// normalizeBLEPayload strips the global enable flag (removed by FW 2.0.0-beta1
// — BLE now auto-activates with scanning) while preserving the rest of the
// payload (rpc, observer, etc.). Saved templates that still set the flag get a
// warning back so the user can clean them up.
func normalizeBLEPayload(payload map[string]interface{}) (map[string]interface{}, string) {
	if payload == nil {
		return nil, ""
	}
	out := map[string]interface{}{}
	var warning string
	for key, val := range payload {
		if strings.EqualFold(key, "enable") {
			warning = "ble.enable stripped — flag removed in firmware 2.0.0-beta1"
			continue
		}
		out[key] = val
	}
	return out, warning
}

// normalizeCoverPayload validates and forwards Cover.SetConfig fields,
// including the slat/tilt sub-object introduced by FW 2.0.0-beta1 for
// venetian-blinds support. Unknown fields pass through so device-specific
// options stay reachable.
func normalizeCoverPayload(payload map[string]interface{}) (map[string]interface{}, string) {
	if payload == nil {
		return nil, ""
	}
	out := map[string]interface{}{}
	for key, val := range payload {
		out[key] = val
	}
	return out, ""
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

func rpcConfigSection(ctx context.Context, client *shellyclient.Client, ip, method string, payload map[string]interface{}, section string) SectionResult {
	return rpcSection(ctx, client, ip, method, map[string]interface{}{"config": payload}, section)
}

func rpcSection(ctx context.Context, client *shellyclient.Client, ip, method string, params map[string]interface{}, section string) SectionResult {
	result, err := client.RPC(ctx, ip, method, params)
	if err == nil {
		restartRequired := false
		if result != nil {
			if v, ok := result["restart_required"].(bool); ok {
				restartRequired = v
			}
		}
		return SectionResult{Section: section, Status: "ok", Detail: method, RestartRequired: restartRequired}
	}
	if shellyclient.IsMethodNotFound(err) {
		return SectionResult{Section: section, Status: "skipped", Detail: "method not supported by this device"}
	}
	if errors.Is(err, shellyclient.ErrAuthRequired) {
		return SectionResult{Section: section, Status: "failed", Detail: "401 Unauthorized"}
	}
	if errors.Is(err, shellyclient.ErrAuthLockout) {
		return SectionResult{Section: section, Status: "failed", Detail: "device locked (brute-force protection)"}
	}
	var rpcErr *shellyclient.RPCError
	if errors.As(err, &rpcErr) {
		detail := rpcErr.Message()
		if code := rpcErr.Code(); code != 0 {
			detail = fmt.Sprintf("%s (%d)", detail, code)
		}
		return SectionResult{Section: section, Status: "failed", Detail: detail}
	}
	return SectionResult{Section: section, Status: "failed", Detail: err.Error()}
}

// isMethodNotFound is preserved for back-compat with user_ca.go.
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

func stringField(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
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
