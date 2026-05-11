package observability

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistryExposesRegisteredCounters(t *testing.T) {
	r := NewRegistry()
	r.RegisterCounter("test_total", "test counter help")
	r.Inc("test_total")
	r.Inc("test_total")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))

	body := rec.Body.String()
	for _, want := range []string{
		"# HELP test_total test counter help",
		"# TYPE test_total counter",
		"test_total 2",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("output missing %q. body=\n%s", want, body)
		}
	}
}

func TestRegistryExposesGauge(t *testing.T) {
	r := NewRegistry()
	r.RegisterGauge("devices_online", "online device count")
	r.Set("devices_online", 42)
	r.Set("devices_online", 40) // gauges overwrite, not accumulate

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "# TYPE devices_online gauge") {
		t.Errorf("missing gauge TYPE line: %s", body)
	}
	if !strings.Contains(body, "devices_online 40") {
		t.Errorf("gauge value not updated: %s", body)
	}
}

func TestLabelledCounterBucketsByLabelSet(t *testing.T) {
	r := NewRegistry()
	r.RegisterLabelledCounter("mcp_calls_total", "MCP tool invocations")
	r.IncLabelled("mcp_calls_total", map[string]string{"tool": "list_devices"})
	r.IncLabelled("mcp_calls_total", map[string]string{"tool": "list_devices"})
	r.IncLabelled("mcp_calls_total", map[string]string{"tool": "refresh_device"})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `mcp_calls_total{tool="list_devices"} 2`) {
		t.Errorf("missing or wrong list_devices bucket: %s", body)
	}
	if !strings.Contains(body, `mcp_calls_total{tool="refresh_device"} 1`) {
		t.Errorf("missing or wrong refresh_device bucket: %s", body)
	}
}

func TestUnregisteredOpsAreNoop(t *testing.T) {
	r := NewRegistry()
	// All three should silently no-op — no panic, no output.
	r.Inc("never_registered")
	r.Set("never_registered", 5)
	r.IncLabelled("never_registered", map[string]string{"x": "y"})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if strings.Contains(rec.Body.String(), "never_registered") {
		t.Errorf("unregistered metric leaked: %s", rec.Body.String())
	}
}

func TestEncodeLabelsEscapesSpecialChars(t *testing.T) {
	got := encodeLabels(map[string]string{"path": `/api/"weird"`, "method": "GET"})
	// Keys must be sorted alphabetically: method before path.
	want := `method="GET",path="/api/\"weird\""`
	if got != want {
		t.Errorf("encodeLabels = %q, want %q", got, want)
	}
}
