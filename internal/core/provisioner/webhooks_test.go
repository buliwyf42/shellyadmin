package provisioner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestWebhooksSection_DriveAllOps covers delete_all → delete → update → create
// in one provisioning pass and asserts the order on the wire.
func TestWebhooksSection_DriveAllOps(t *testing.T) {
	var mu sync.Mutex
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shelly":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "wh-test", "gen": 2, "id": "abc"})
			return
		case "/rpc":
			var body struct {
				Method string `json:"method"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			calls = append(calls, body.Method)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "result": map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ip := server.Listener.Addr().String()
	template := map[string]interface{}{
		"webhooks": map[string]interface{}{
			"delete_all": true,
			"delete":     []interface{}{float64(1), float64(2)},
			"update":     []interface{}{map[string]interface{}{"id": float64(3), "name": "renamed"}},
			"create": []interface{}{
				map[string]interface{}{"cid": float64(0), "event": "switch.on", "urls": []interface{}{"http://a"}},
				map[string]interface{}{"cid": float64(0), "event": "switch.off", "urls": []interface{}{"http://b"}},
			},
		},
	}

	_, results := ProvisionDevice(context.Background(), ip, template, time.Second)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(results), results)
	}
	if results[0].Status != "ok" {
		t.Fatalf("expected ok, got %#v", results[0])
	}

	mu.Lock()
	defer mu.Unlock()
	// Drop the Shelly.GetConfig call that resolvedDeviceName issues.
	got := []string{}
	for _, m := range calls {
		if strings.HasPrefix(m, "Webhook.") {
			got = append(got, m)
		}
	}
	want := []string{
		"Webhook.DeleteAll",
		"Webhook.Delete",
		"Webhook.Delete",
		"Webhook.Update",
		"Webhook.Create",
		"Webhook.Create",
	}
	if !slicesEqual(got, want) {
		t.Errorf("rpc order = %v, want %v", got, want)
	}
}

// TestWebhooksSection_MethodNotFound is the legacy-firmware path: device
// returns 404 on Webhook.Create. We expect a "skipped" result rather than a
// noisy failure so mixed-fleet templates don't blow up.
func TestWebhooksSection_MethodNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shelly":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "old", "gen": 2, "id": "z"})
			return
		case "/rpc":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 1, "error": map[string]any{"code": float64(404), "message": "not found"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ip := server.Listener.Addr().String()
	template := map[string]interface{}{
		"webhooks": map[string]interface{}{
			"create": []interface{}{
				map[string]interface{}{"cid": float64(0), "event": "switch.on"},
			},
		},
	}
	_, results := ProvisionDevice(context.Background(), ip, template, time.Second)
	if len(results) != 1 {
		t.Fatalf("got %d results", len(results))
	}
	if results[0].Status != "skipped" {
		t.Errorf("expected skipped on 404, got %#v", results[0])
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
