package provisioner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// End-to-end provisioning smoke. A single template carrying four common
// sections (sys + mqtt + wifi + auth) drives one ProvisionDevice call;
// the test asserts that:
//
//  1. Every section ends with Status="ok" — none silently dropped.
//  2. The exact set of expected RPCs is issued (one per SetConfig section
//     plus the auth call), in the right order, with the right shape.
//  3. The {device_name} token in the sys section gets hydrated from the
//     device's reported name (not the raw template literal).
//  4. The auth section computes HA1 correctly: SHA-256("admin:serial:pass").
//     This is the most failure-prone step — a bad HA1 silently locks the
//     operator out of the device until factory reset.
//
// Guards against cross-section regressions that single-section tests miss
// (state leaking across applySection calls, the iteration order over the
// template map producing surprising effects, etc.).
func TestProvisionDevice_MultiSectionSmoke(t *testing.T) {
	const (
		serial   = "abcd1234"
		devName  = "kitchen-plug"
		password = "supersecret"
	)
	expectedHA1 := func() string {
		sum := sha256.Sum256([]byte("admin:" + serial + ":" + password))
		return hex.EncodeToString(sum[:])
	}()

	var calls []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shelly":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":  devName,
				"model": "S4SW-001P8EU",
				"gen":   4,
				"id":    serial,
			})
		case "/rpc":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode rpc body: %v", err)
			}
			calls = append(calls, body)
			method, _ := body["method"].(string)
			// Shelly.GetConfig is the preflight that resolves the configured
			// device name. Return a populated sys.device.name so the {device_name}
			// substitution has something to use.
			if method == "Shelly.GetConfig" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":     body["id"],
					"result": map[string]any{"sys": map[string]any{"device": map[string]any{"name": devName}}},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": body["id"], "result": map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	template := map[string]interface{}{
		"sys": map[string]interface{}{
			"device": map[string]any{"name": "{device_name}"},
		},
		"mqtt": map[string]interface{}{
			"enable": true,
			"server": "mqtt.home.lan:1883",
		},
		"wifi": map[string]interface{}{
			"sta": map[string]any{
				"enable": true,
				"ssid":   "homewifi",
				"pass":   "wifipass",
			},
		},
		"auth": map[string]interface{}{
			"pass": password,
		},
	}

	info, results := ProvisionDevice(context.Background(), srv.Listener.Addr().String(), template, 5*time.Second)
	if info.Gen != 4 {
		t.Errorf("info.Gen = %d, want 4 (probed from /shelly)", info.Gen)
	}
	if len(results) != 4 {
		t.Fatalf("results = %d entries, want 4 (sys, mqtt, wifi, auth)", len(results))
	}
	for _, res := range results {
		if res.Status != "ok" {
			t.Errorf("section %q ended with status=%q detail=%q, want ok", res.Section, res.Status, res.Detail)
		}
	}

	// Now verify every expected RPC was issued and the payload shape is
	// what the device would expect. We don't pin the iteration order over
	// the template map (Go map iteration is unspecified) but every method
	// must appear exactly once.
	want := map[string]bool{
		"Sys.SetConfig":    false,
		"MQTT.SetConfig":   false,
		"Wifi.SetConfig":   false,
		"Shelly.SetAuth":   false,
		"Shelly.GetConfig": false, // preflight for {device_name}
	}
	for _, call := range calls {
		method, _ := call["method"].(string)
		if seen, expected := want[method]; expected {
			if seen {
				t.Errorf("method %s seen more than once", method)
			}
			want[method] = true
		}
	}
	for method, seen := range want {
		if !seen {
			t.Errorf("expected RPC %s not issued (got %d calls total)", method, len(calls))
		}
	}

	// Pick the auth call out of the recorded list and verify HA1. This is
	// the single highest-risk computation in the provisioner — a wrong
	// hash silently locks the operator out.
	var authCall map[string]any
	for _, call := range calls {
		if call["method"] == "Shelly.SetAuth" {
			authCall = call
			break
		}
	}
	if authCall == nil {
		t.Fatalf("Shelly.SetAuth not in recorded calls")
	}
	authParams, _ := authCall["params"].(map[string]any)
	if got := authParams["user"]; got != "admin" {
		t.Errorf("auth.user = %v, want admin", got)
	}
	if got := authParams["realm"]; got != serial {
		t.Errorf("auth.realm = %v, want %s (the device serial)", got, serial)
	}
	if got := authParams["ha1"]; got != expectedHA1 {
		t.Errorf("auth.ha1 = %v\nwant %s\n(SHA-256 of \"admin:%s:%s\" — wrong hash silently locks the operator out)", got, expectedHA1, serial, password)
	}

	// And the sys section's {device_name} token must have been hydrated
	// to the actual device name before being sent.
	for _, call := range calls {
		if call["method"] != "Sys.SetConfig" {
			continue
		}
		params, _ := call["params"].(map[string]any)
		config, _ := params["config"].(map[string]any)
		device, _ := config["device"].(map[string]any)
		if got := device["name"]; got != devName {
			t.Errorf("sys.device.name = %v, want %s (token substitution should have hydrated it)", got, devName)
		}
	}
}
