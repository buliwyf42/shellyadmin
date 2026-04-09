package provisioner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type DeviceInfo struct {
	Name  string `json:"name"`
	Model string `json:"model"`
	FW    string `json:"fw"`
	Gen   int    `json:"gen"`
	IP    string `json:"ip"`
}

type SectionResult struct {
	Section string `json:"section"`
	Status  string `json:"status"`
	Detail  string `json:"detail"`
}

func ProvisionDevice(ctx context.Context, ip string, template map[string]interface{}, timeout time.Duration) (DeviceInfo, []SectionResult) {
	client := &http.Client{Timeout: timeout}
	info, serial := identify(ctx, client, ip)
	name := info.Name
	if name == "" {
		name = firstNonEmpty(serial, ip)
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
	return DeviceInfo{Name: base.Name, Model: base.Model, FW: base.FW, Gen: base.Gen, IP: ip}, firstNonEmpty(base.ID, base.MAC)
}

func applySection(ctx context.Context, client *http.Client, ip string, gen int, serial, section string, raw interface{}) SectionResult {
	payload, _ := raw.(map[string]interface{})
	switch strings.ToLower(section) {
	case "gen2_rpc":
		if gen == 1 {
			return SectionResult{Section: section, Status: "skipped", Detail: "gen2+ only"}
		}
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
		if gen != 1 {
			return SectionResult{Section: section, Status: "skipped", Detail: "gen1 only"}
		}
		for endpoint, item := range payload {
			params, ok := item.(map[string]interface{})
			if !ok {
				return SectionResult{Section: section, Status: "failed", Detail: "endpoint params must be object"}
			}
			result := gen1HTTPSection(ctx, client, ip, endpoint, params, section)
			if result.Status != "ok" {
				return result
			}
		}
		return SectionResult{Section: section, Status: "ok", Detail: "gen1 endpoints applied"}
	case "mqtt":
		if gen == 1 {
			return gen1HTTPSection(ctx, client, ip, "settings/mqtt", payload, section)
		}
		return rpcSection(ctx, client, ip, "MQTT.SetConfig", payload, section)
	case "sys":
		if gen == 1 {
			return gen1HTTPSection(ctx, client, ip, "settings", payload, section)
		}
		return rpcSection(ctx, client, ip, "Sys.SetConfig", payload, section)
	case "ws":
		if gen == 1 {
			return SectionResult{Section: section, Status: "skipped", Detail: "gen2+ only"}
		}
		return rpcSection(ctx, client, ip, "WS.SetConfig", payload, section)
	case "ble":
		if gen == 1 {
			return SectionResult{Section: section, Status: "skipped", Detail: "gen2+ only"}
		}
		return rpcSection(ctx, client, ip, "BLE.SetConfig", payload, section)
	case "matter":
		if gen == 1 {
			return SectionResult{Section: section, Status: "skipped", Detail: "gen2+ only"}
		}
		return rpcSection(ctx, client, ip, "Matter.SetConfig", payload, section)
	case "cloud":
		if gen == 1 {
			return SectionResult{Section: section, Status: "skipped", Detail: "gen2+ only"}
		}
		return rpcSection(ctx, client, ip, "Cloud.SetConfig", payload, section)
	case "wifi":
		if gen == 1 {
			return SectionResult{Section: section, Status: "skipped", Detail: "gen2+ only"}
		}
		return rpcSection(ctx, client, ip, "Wifi.SetConfig", payload, section)
	case "kvs":
		if gen == 1 {
			return SectionResult{Section: section, Status: "skipped", Detail: "gen2+ only"}
		}
		for key, val := range payload {
			result := rpcSection(ctx, client, ip, "KVS.Set", map[string]interface{}{"key": key, "value": val}, section)
			if result.Status != "ok" {
				return result
			}
		}
		return SectionResult{Section: section, Status: "ok", Detail: "keys written"}
	case "ota":
		if gen == 1 {
			return SectionResult{Section: section, Status: "skipped", Detail: "ota templating not supported for gen1 here"}
		}
		return rpcSection(ctx, client, ip, "Shelly.Update", payload, section)
	case "auth":
		if gen == 1 {
			return SectionResult{Section: section, Status: "skipped", Detail: "gen2+ only"}
		}
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
		if gen == 1 {
			return SectionResult{Section: section, Status: "skipped", Detail: "generic config unsupported on gen1"}
		}
		method := strings.ToUpper(section[:1]) + section[1:] + ".SetConfig"
		return rpcSection(ctx, client, ip, method, payload, section)
	}
}

func gen1HTTPSection(ctx context.Context, client *http.Client, ip, endpoint string, payload map[string]interface{}, section string) SectionResult {
	values := url.Values{}
	for key, raw := range payload {
		values.Set(key, gen1Value(raw))
	}
	target := "http://" + ip + "/" + strings.TrimPrefix(endpoint, "/")
	if encoded := values.Encode(); encoded != "" {
		target += "?" + encoded
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	resp, err := client.Do(req)
	if err != nil {
		return SectionResult{Section: section, Status: "failed", Detail: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return SectionResult{Section: section, Status: "failed", Detail: resp.Status}
	}
	return SectionResult{Section: section, Status: "ok", Detail: endpoint}
}

func gen1Value(v interface{}) string {
	switch value := v.(type) {
	case bool:
		if value {
			return "true"
		}
		return "false"
	case string:
		return value
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
	default:
		return fmt.Sprint(value)
	}
}

func rpcSection(ctx context.Context, client *http.Client, ip, method string, payload map[string]interface{}, section string) SectionResult {
	buf, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+ip+"/rpc/"+method, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return SectionResult{Section: section, Status: "failed", Detail: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return SectionResult{Section: section, Status: "failed", Detail: resp.Status}
	}
	return SectionResult{Section: section, Status: "ok", Detail: method}
}

func substitute(v interface{}, name string) interface{} {
	switch val := v.(type) {
	case string:
		value := strings.ReplaceAll(val, "{device_name}", name)
		if strings.HasPrefix(value, "${ENV:") && strings.HasSuffix(value, "}") {
			key := strings.TrimSuffix(strings.TrimPrefix(value, "${ENV:"), "}")
			return os.Getenv(key)
		}
		return value
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
