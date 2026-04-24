package provisioner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProvisionDevice_Gen2SetConfigUsesJSONRPCEnvelope(t *testing.T) {
	var calls []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shelly":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":  "test-switch",
				"model": "S4SW-001P8EU",
				"gen":   4,
				"id":    "abcd1234",
			})
		case "/rpc":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode rpc body: %v", err)
			}
			calls = append(calls, body)
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ip := server.Listener.Addr().String()
	template := map[string]interface{}{
		"sys": map[string]interface{}{
			"device": map[string]any{"name": "{device_name}"},
		},
	}

	_, results := ProvisionDevice(context.Background(), ip, template, time.Second)
	if len(results) != 1 || results[0].Status != "ok" {
		t.Fatalf("expected successful result, got %#v", results)
	}
	// Gen2+ provisioning makes a preflight Shelly.GetConfig call to resolve the
	// configured device name, then the actual SetConfig call — find it by method.
	var sysCall map[string]any
	for _, call := range calls {
		if call["method"] == "Sys.SetConfig" {
			sysCall = call
			break
		}
	}
	if sysCall == nil {
		t.Fatalf("Sys.SetConfig call not found among %d rpc calls", len(calls))
	}
	params, ok := sysCall["params"].(map[string]any)
	if !ok {
		t.Fatalf("params missing or wrong type: %#v", sysCall["params"])
	}
	config, ok := params["config"].(map[string]any)
	if !ok {
		t.Fatalf("config missing or wrong type: %#v", params["config"])
	}
	device, ok := config["device"].(map[string]any)
	if !ok || device["name"] != "test-switch" {
		t.Fatalf("device config = %#v, want hydrated device name", config["device"])
	}
}

func TestProvisionDevice_SurfacesRPCErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shelly":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "test-switch",
				"gen":  4,
				"id":   "abcd1234",
			})
		case "/rpc":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    500,
					"message": "bad config",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ip := server.Listener.Addr().String()
	template := map[string]interface{}{
		"mqtt": map[string]interface{}{
			"enable": true,
		},
	}

	_, results := ProvisionDevice(context.Background(), ip, template, time.Second)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %#v", results)
	}
	if results[0].Status != "failed" {
		t.Fatalf("expected failed result, got %#v", results[0])
	}
	if results[0].Detail != "bad config (500)" {
		t.Fatalf("detail = %q, want rpc error message", results[0].Detail)
	}
}

func TestProvisionDevice_UsesConfiguredNameBeforeIPFallback(t *testing.T) {
	var calls []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shelly":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "192.168.1.20",
				"gen":  4,
				"id":   "abcd1234",
			})
		case "/rpc":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode rpc body: %v", err)
			}
			calls = append(calls, body)
			method, _ := body["method"].(string)
			if method == "Shelly.GetConfig" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"result": map[string]any{
						"sys": map[string]any{
							"device": map[string]any{"name": "kitchen-switch"},
						},
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ip := server.Listener.Addr().String()
	template := map[string]interface{}{
		"mqtt": map[string]interface{}{
			"client_id": "{device_name}",
		},
	}

	_, results := ProvisionDevice(context.Background(), ip, template, time.Second)
	if len(results) != 1 || results[0].Status != "ok" {
		t.Fatalf("expected successful result, got %#v", results)
	}
	if len(calls) < 2 {
		t.Fatalf("expected config lookup and setter rpc calls, got %d", len(calls))
	}
	params, ok := calls[1]["params"].(map[string]any)
	if !ok {
		t.Fatalf("params missing or wrong type: %#v", calls[1]["params"])
	}
	config, ok := params["config"].(map[string]any)
	if !ok {
		t.Fatalf("config missing or wrong type: %#v", params["config"])
	}
	if got := config["client_id"]; got != "kitchen-switch" {
		t.Fatalf("client_id = %#v, want configured device name", got)
	}
}

func TestProvisionDevice_MethodNotFoundBecomesSkipped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shelly":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "test-switch",
				"gen":  4,
				"id":   "abcd1234",
			})
		case "/rpc":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			method, _ := body["method"].(string)
			if method == "Shelly.GetConfig" {
				_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{}})
				return
			}
			// Simulate a device that does not support BLE.SetConfig.
			// Shelly uses non-standard JSON-RPC error code 404 (not -32601) for
			// unsupported methods.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    float64(404),
					"message": "Not Found",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ip := server.Listener.Addr().String()
	template := map[string]interface{}{
		"ble": map[string]interface{}{
			"gateway": map[string]any{"enable": true},
		},
	}

	_, results := ProvisionDevice(context.Background(), ip, template, time.Second)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %#v", results)
	}
	if results[0].Status != "skipped" {
		t.Fatalf("expected skipped for method-not-found, got status=%q detail=%q", results[0].Status, results[0].Detail)
	}
	if results[0].Detail != "method not supported by this device" {
		t.Fatalf("detail = %q, want method not supported message", results[0].Detail)
	}
}

func TestNormalizeSysPayload_FlatLonAccepted(t *testing.T) {
	payload := map[string]interface{}{
		"lon": 13.405,
		"lat": 52.52,
	}
	out, _ := normalizeSysPayload(payload)
	location, ok := out["location"].(map[string]interface{})
	if !ok {
		t.Fatalf("location not set: %#v", out)
	}
	if location["lon"] != 13.405 {
		t.Fatalf("location.lon = %v, want 13.405", location["lon"])
	}
}

func TestNormalizeSysPayload_DebugMQTTPassthrough(t *testing.T) {
	payload := map[string]interface{}{
		"debug": map[string]interface{}{
			"mqtt": map[string]interface{}{"enable": true},
		},
	}
	out, _ := normalizeSysPayload(payload)
	debug, ok := out["debug"].(map[string]interface{})
	if !ok {
		t.Fatalf("debug not set: %#v", out)
	}
	mqtt, ok := debug["mqtt"].(map[string]interface{})
	if !ok {
		t.Fatalf("debug.mqtt not set: %#v", debug)
	}
	if mqtt["enable"] != true {
		t.Fatalf("debug.mqtt.enable = %v, want true", mqtt["enable"])
	}
}

func TestNormalizeSysPayload_ProfileAndAddonTypePassthrough(t *testing.T) {
	payload := map[string]interface{}{
		"profile":    "cover",
		"addon_type": "temperature",
	}
	out, _ := normalizeSysPayload(payload)
	if out["profile"] != "cover" {
		t.Fatalf("profile = %v, want cover", out["profile"])
	}
	if out["addon_type"] != "temperature" {
		t.Fatalf("addon_type = %v, want temperature", out["addon_type"])
	}
}

func TestSubstitute_EnvTokensArePreservedLiterally(t *testing.T) {
	t.Setenv("SHELLYADMIN_PASS_HASH", "argon2id$should-never-leak")
	input := map[string]interface{}{
		"sys": map[string]interface{}{
			"device": map[string]interface{}{
				"name": "${ENV:SHELLYADMIN_PASS_HASH}",
			},
		},
		"mqtt": map[string]interface{}{
			"pass": "prefix-${ENV:SHELLYADMIN_PASS_HASH}-suffix",
		},
	}
	out := substitute(input, "device-01").(map[string]interface{})
	sys := out["sys"].(map[string]interface{})
	device := sys["device"].(map[string]interface{})
	if got := device["name"]; got != "${ENV:SHELLYADMIN_PASS_HASH}" {
		t.Fatalf("sys.device.name = %q, want literal token", got)
	}
	mqtt := out["mqtt"].(map[string]interface{})
	if got := mqtt["pass"]; got != "prefix-${ENV:SHELLYADMIN_PASS_HASH}-suffix" {
		t.Fatalf("mqtt.pass = %q, want literal token", got)
	}
}

func TestSubstitute_DeviceNameTokenIsReplaced(t *testing.T) {
	input := map[string]interface{}{
		"sys": map[string]interface{}{
			"device": map[string]interface{}{
				"name": "shelly-{device_name}",
			},
		},
	}
	out := substitute(input, "kitchen").(map[string]interface{})
	sys := out["sys"].(map[string]interface{})
	device := sys["device"].(map[string]interface{})
	if got := device["name"]; got != "shelly-kitchen" {
		t.Fatalf("device.name = %q, want shelly-kitchen", got)
	}
}

func TestProvisionDevice_WSIgnoresTLSModeForPlainWS(t *testing.T) {
	var call map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shelly":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "test-switch",
				"gen":  4,
				"id":   "abcd1234",
			})
		case "/rpc":
			if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
				t.Fatalf("decode rpc body: %v", err)
			}
			method, _ := call["method"].(string)
			if method == "Shelly.GetConfig" {
				_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ip := server.Listener.Addr().String()
	template := map[string]interface{}{
		"ws": map[string]interface{}{
			"enable":   true,
			"server":   "ws://example.invalid/ws",
			"tls_mode": "no_validation",
		},
	}

	_, results := ProvisionDevice(context.Background(), ip, template, time.Second)
	if len(results) != 1 || results[0].Status != "ok" {
		t.Fatalf("expected successful result, got %#v", results)
	}
	params := call["params"].(map[string]any)
	config := params["config"].(map[string]any)
	if _, ok := config["ssl_ca"]; ok {
		t.Fatalf("ws config unexpectedly included ssl_ca: %#v", config)
	}
	if results[0].Detail != "WS.SetConfig; ws TLS mode ignored because ws.server is non-TLS" {
		t.Fatalf("detail = %q, want non-tls warning", results[0].Detail)
	}
}

func TestProvisionDevice_SurfacesRestartRequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shelly":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "test-switch",
				"gen":  4,
				"id":   "abcd1234",
			})
		case "/rpc":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			method, _ := body["method"].(string)
			if method == "Shelly.GetConfig" {
				_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"restart_required": true},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ip := server.Listener.Addr().String()
	template := map[string]interface{}{
		"sys": map[string]interface{}{
			"device": map[string]any{"name": "mydevice"},
		},
	}

	_, results := ProvisionDevice(context.Background(), ip, template, time.Second)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "ok" {
		t.Fatalf("expected ok status, got %q", results[0].Status)
	}
	if !results[0].RestartRequired {
		t.Fatal("expected RestartRequired=true, got false")
	}
}
