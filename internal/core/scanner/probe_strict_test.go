package scanner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// nopLog discards scanner debug output for tests.
func nopLog(_ string, _ string) {}

// TestProbe_RejectsNonShellyJSONResponse covers the UniFi UDM / Protect class:
// an HTTP 200 with a JSON body that doesn't carry any Shelly identifiers
// (no `mac`, no `gen`). Old code would have created a Device with empty
// fields. The fix requires either mac or gen to be present.
func TestProbe_RejectsNonShellyJSONResponse(t *testing.T) {
	cases := []struct {
		name string
		body any
	}{
		{
			// UniFi-style response — generic API envelope, no Shelly markers.
			name: "unifi-envelope",
			body: map[string]any{
				"meta": map[string]any{"rc": "ok"},
				"data": []any{},
			},
		},
		{
			// Empty JSON object — common health-check response.
			name: "empty-object",
			body: map[string]any{},
		},
		{
			// Object with only `name` (a UniFi camera might have a hostname here).
			name: "name-only",
			body: map[string]any{"name": "UniFi-Protect-G4"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(tc.body)
			}))
			defer srv.Close()

			ip := srv.Listener.Addr().String()
			dev := ProbeDeviceWithOptions(context.Background(), ip, ProbeOptions{Timeout: time.Second}, nopLog)
			if dev != nil {
				t.Errorf("expected nil device for non-Shelly response, got %#v", dev)
			}
		})
	}
}

// TestProbe_AcceptsRealShellyResponse confirms the validator doesn't reject
// the legitimate path. A real Shelly always reports `gen` (Gen2+) or `mac`.
func TestProbe_AcceptsRealShellyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shelly":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"gen":   2,
				"model": "SNSW-001P16EU",
				"mac":   "AA:BB:CC:DD:EE:FF",
				"fw":    "1.2.3",
			})
		case "/rpc":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "result": map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ip := srv.Listener.Addr().String()
	dev := ProbeDeviceWithOptions(context.Background(), ip, ProbeOptions{Timeout: time.Second}, nopLog)
	if dev == nil {
		t.Fatal("expected non-nil device for real Shelly response")
	}
	if dev.MAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("MAC = %q", dev.MAC)
	}
	if dev.Gen != 2 {
		t.Errorf("Gen = %d", dev.Gen)
	}
}

// TestProbe_RejectsEmpty200 covers the case the v0.1.0 regression hit:
// a server that answers 200 with no body. Now the probe path returns nil
// rather than a junk Device.
func TestProbe_RejectsEmpty200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// No body.
	}))
	defer srv.Close()

	ip := srv.Listener.Addr().String()
	dev := ProbeDeviceWithOptions(context.Background(), ip, ProbeOptions{Timeout: time.Second}, nopLog)
	if dev != nil {
		t.Errorf("expected nil device for empty 200, got %#v", dev)
	}
}

// TestProbe_AcceptsMACOnly confirms that a Gen1 (or Gen0/legacy probe) device
// that returns mac without gen still creates a Device — gen will default to 2
// downstream. We explicitly support this path because some early Gen2
// firmwares omitted `gen` from /shelly until 0.10.x.
func TestProbe_AcceptsMACOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shelly":
			_ = json.NewEncoder(w).Encode(map[string]any{"mac": "AABBCCDDEEFF"})
		case "/rpc":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "result": map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ip := srv.Listener.Addr().String()
	dev := ProbeDeviceWithOptions(context.Background(), ip, ProbeOptions{Timeout: time.Second}, nopLog)
	if dev == nil {
		t.Fatal("expected device for mac-only response")
	}
	if dev.MAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("MAC = %q (want normalized form)", dev.MAC)
	}
}
