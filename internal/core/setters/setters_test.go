package setters

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type captured struct {
	method string
	body   map[string]any
}

func newCaptureServer(t *testing.T, status int) (*httptest.Server, *captured) {
	t.Helper()
	cap := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("HTTP method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/rpc" {
			t.Errorf("path = %q, want /rpc", r.URL.Path)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("unmarshal body: %v (raw=%s)", err, string(raw))
		}
		if m, ok := payload["method"].(string); ok {
			cap.method = m
		}
		cap.body = payload
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

func hostFromURL(t *testing.T, url string) string {
	t.Helper()
	host := strings.TrimPrefix(url, "http://")
	if host == url {
		t.Fatalf("unexpected URL scheme: %q", url)
	}
	return host
}

func TestSetTimezoneSendsSysSetConfigWithLocationTZ(t *testing.T) {
	srv, cap := newCaptureServer(t, http.StatusOK)
	if !SetTimezone(context.Background(), hostFromURL(t, srv.URL), "Europe/Berlin", 2, 2*time.Second) {
		t.Fatalf("SetTimezone returned false")
	}
	if cap.method != "Sys.SetConfig" {
		t.Fatalf("method = %q, want Sys.SetConfig", cap.method)
	}
	params, _ := cap.body["params"].(map[string]any)
	config, _ := params["config"].(map[string]any)
	location, _ := config["location"].(map[string]any)
	if got := location["tz"]; got != "Europe/Berlin" {
		t.Fatalf("location.tz = %v, want Europe/Berlin", got)
	}
}

func TestSetMQTTEnabledSendsBoolFlag(t *testing.T) {
	srv, cap := newCaptureServer(t, http.StatusOK)
	if !SetMQTTEnabled(context.Background(), hostFromURL(t, srv.URL), true, 2, 2*time.Second) {
		t.Fatalf("SetMQTTEnabled returned false")
	}
	if cap.method != "MQTT.SetConfig" {
		t.Fatalf("method = %q, want MQTT.SetConfig", cap.method)
	}
	params, _ := cap.body["params"].(map[string]any)
	config, _ := params["config"].(map[string]any)
	if got := config["enable"]; got != true {
		t.Fatalf("config.enable = %v, want true", got)
	}
}

func TestSetCloudEnabledSendsCloudSetConfig(t *testing.T) {
	srv, cap := newCaptureServer(t, http.StatusOK)
	if !SetCloudEnabled(context.Background(), hostFromURL(t, srv.URL), false, 2, 2*time.Second) {
		t.Fatalf("SetCloudEnabled returned false")
	}
	if cap.method != "Cloud.SetConfig" {
		t.Fatalf("method = %q, want Cloud.SetConfig", cap.method)
	}
	params, _ := cap.body["params"].(map[string]any)
	config, _ := params["config"].(map[string]any)
	if got := config["enable"]; got != false {
		t.Fatalf("config.enable = %v, want false", got)
	}
}

func TestSetBLEEnabledSendsBLESetConfig(t *testing.T) {
	srv, cap := newCaptureServer(t, http.StatusOK)
	if !SetBLEEnabled(context.Background(), hostFromURL(t, srv.URL), true, 2, 2*time.Second) {
		t.Fatalf("SetBLEEnabled returned false")
	}
	if cap.method != "BLE.SetConfig" {
		t.Fatalf("method = %q, want BLE.SetConfig", cap.method)
	}
}

func TestRebootSendsShellyRebootWithoutConfigWrap(t *testing.T) {
	srv, cap := newCaptureServer(t, http.StatusOK)
	if !Reboot(context.Background(), hostFromURL(t, srv.URL), 2, 2*time.Second) {
		t.Fatalf("Reboot returned false")
	}
	if cap.method != "Shelly.Reboot" {
		t.Fatalf("method = %q, want Shelly.Reboot", cap.method)
	}
	params, _ := cap.body["params"].(map[string]any)
	if _, hasConfig := params["config"]; hasConfig {
		t.Fatalf("Shelly.Reboot params should not be wrapped in config: %v", params)
	}
}

func TestSettersReturnFalseOn5xx(t *testing.T) {
	srv, _ := newCaptureServer(t, http.StatusInternalServerError)
	if SetMQTTEnabled(context.Background(), hostFromURL(t, srv.URL), true, 2, 2*time.Second) {
		t.Fatalf("SetMQTTEnabled returned true on 500")
	}
}
