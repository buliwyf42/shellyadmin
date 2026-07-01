package services

import (
	"testing"

	"shellyadmin/internal/models"
)

// Real sys.alt payload shape from a Power Strip Gen4 (Zigbee variant) and a
// Pro 3EM (Pro Sensor Addon variant), plus a device without alt.
func TestSysAltVariants(t *testing.T) {
	powerStrip := `{"sys":{"alt":{"PowerStripZB":{"name":"Shelly Power Strip Gen4","desc":"Shelly Power Strip Gen4 with Zigbee","beta":{"version":"2.0.0-beta3","build_id":"x"},"stable":{"version":"1.7.99-powerstripg4prod1","build_id":"y"}}}}}`
	dev := models.Device{RawStatus: powerStrip}
	got := sysAltVariants(dev)
	if len(got) != 1 {
		t.Fatalf("want 1 variant, got %d", len(got))
	}
	v := got[0]
	if v.ID != "PowerStripZB" || v.Name != "Shelly Power Strip Gen4" {
		t.Errorf("bad id/name: %+v", v)
	}
	if v.Stable != "1.7.99-powerstripg4prod1" || v.Beta != "2.0.0-beta3" {
		t.Errorf("bad versions: stable=%q beta=%q", v.Stable, v.Beta)
	}

	// Beta-only variant (Pro 3EM Pro Sensor Addon has no stable channel).
	betaOnly := `{"sys":{"alt":{"Pro3EMProAddon":{"name":"Shelly Pro 3 EM","desc":"Pro 3 EM with Pro Sensor Addon","beta":{"version":"2.0.0-beta3","build_id":"z"}}}}}`
	got = sysAltVariants(models.Device{RawStatus: betaOnly})
	if len(got) != 1 || got[0].Beta != "2.0.0-beta3" || got[0].Stable != "" {
		t.Errorf("beta-only parse wrong: %+v", got)
	}

	// No alt object → nil, no crash.
	if got := sysAltVariants(models.Device{RawStatus: `{"sys":{"mac":"aa"}}`}); got != nil {
		t.Errorf("want nil for no-alt device, got %+v", got)
	}
	// Empty / garbage RawStatus → nil, no crash.
	if got := sysAltVariants(models.Device{RawStatus: ""}); got != nil {
		t.Errorf("want nil for empty RawStatus, got %+v", got)
	}
	if got := sysAltVariants(models.Device{RawStatus: "not json"}); got != nil {
		t.Errorf("want nil for garbage RawStatus, got %+v", got)
	}
}

func TestSysProvisioning(t *testing.T) {
	// Present → returned as-is.
	dev := models.Device{RawStatus: `{"sys":{"provisioning":{"state":"locked"}}}`}
	if p := sysProvisioning(dev); p == nil || p["state"] != "locked" {
		t.Errorf("want provisioning map, got %+v", p)
	}
	// Absent (fleet default) → nil.
	if p := sysProvisioning(models.Device{RawStatus: `{"sys":{"mac":"aa"}}`}); p != nil {
		t.Errorf("want nil when provisioning absent, got %+v", p)
	}
}
