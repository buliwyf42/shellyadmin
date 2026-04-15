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
	if len(calls) != 1 {
		t.Fatalf("expected 1 rpc call, got %d", len(calls))
	}
	if got := calls[0]["method"]; got != "Sys.SetConfig" {
		t.Fatalf("method = %v, want Sys.SetConfig", got)
	}
	params, ok := calls[0]["params"].(map[string]any)
	if !ok {
		t.Fatalf("params missing or wrong type: %#v", calls[0]["params"])
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
