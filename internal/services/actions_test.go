package services

import (
	"testing"

	"shellyadmin/internal/models"
)

// TestMethodsCovered exercises the three branches of the catalog gate:
// nothing-required, no-cache-yet (rollout window), and the strict
// supported-set filter.
func TestMethodsCovered(t *testing.T) {
	tests := []struct {
		name      string
		supported []string
		required  []string
		probed    bool
		want      bool
	}{
		{"empty required is always covered (probed)", []string{"X"}, nil, true, true},
		{"empty required is always covered (unprobed)", nil, nil, false, true},
		{"unprobed device assumes covered to preserve fallback", nil, []string{"Shelly.Update"}, false, true},
		{"probed + supported", []string{"Shelly.Update", "Shelly.Reboot"}, []string{"Shelly.Update"}, true, true},
		{"probed + missing one", []string{"Shelly.Reboot"}, []string{"Shelly.Update"}, true, false},
		{"probed + missing all", []string{"Shelly.Reboot"}, []string{"Shelly.Update", "Wifi.Scan"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := map[string]bool{}
			for _, m := range tt.supported {
				set[m] = true
			}
			if got := methodsCovered(set, tt.required, tt.probed); got != tt.want {
				t.Errorf("methodsCovered(supported=%v, required=%v, probed=%v) = %v, want %v",
					tt.supported, tt.required, tt.probed, got, tt.want)
			}
		})
	}
}

// TestDescribeAvailableActions_FallbackUnprobed proves that a device with
// no SupportedMethods cache (= never firmware-checked since v0.1.8) still
// gets the full action surface. This is the rollout-window contract.
func TestDescribeAvailableActions_FallbackUnprobed(t *testing.T) {
	device := models.Device{MAC: "AA", Online: true, Gen: 2}
	got := describeDeviceActions(device)
	if len(got) != len(actionCatalog) {
		t.Fatalf("unprobed device action count = %d, want all %d catalog entries",
			len(got), len(actionCatalog))
	}
}

// TestDescribeAvailableActions_FilterByMethods proves that when the cache
// is populated the catalog filter actually drops actions the device can't
// do. A minimal set ("Shelly.Reboot" only) should leave only refresh +
// reboot.
func TestDescribeAvailableActions_FilterByMethods(t *testing.T) {
	device := models.Device{
		MAC:              "AA",
		Online:           true,
		Gen:              2,
		SupportedMethods: []string{"Shelly.Reboot"},
	}
	got := describeDeviceActions(device)
	ids := make(map[string]bool, len(got))
	for _, a := range got {
		ids[a.ID] = true
	}
	if !ids["refresh"] {
		t.Errorf("refresh missing — should always be available regardless of methods")
	}
	if !ids["reboot"] {
		t.Errorf("reboot missing — Shelly.Reboot is in the supported set")
	}
	for _, banned := range []string{"firmware_check", "firmware_update", "wifi_scan", "factory_reset"} {
		if ids[banned] {
			t.Errorf("%s should be filtered out — its required methods aren't in the supported set", banned)
		}
	}
}

// TestDescribeAvailableActions_OnlineGate covers the offline / auth gate.
// Actions with RequiresOnline still appear in the list (so operators see
// what would be available) but are flagged Supported=false with a reason.
func TestDescribeAvailableActions_OnlineGate(t *testing.T) {
	device := models.Device{MAC: "AA", Online: false, Gen: 2}
	got := describeDeviceActions(device)
	for _, a := range got {
		if a.ID == "refresh" {
			if !a.Supported {
				t.Errorf("refresh should stay available even when offline, got Supported=false reason=%q", a.Reason)
			}
			continue
		}
		if a.RequiresOnline && a.Supported {
			t.Errorf("action %q is RequiresOnline but Supported=true on an offline device", a.ID)
		}
	}
}

// TestDescribeAvailableActions_RiskOrdering pins the risk-grouped output
// order: low risks come first, then medium, then high. A frontend that
// renders in returned order then reads as a natural progression from
// "click freely" to "type the device name first".
func TestDescribeAvailableActions_RiskOrdering(t *testing.T) {
	device := models.Device{MAC: "AA", Online: true, Gen: 2}
	got := describeDeviceActions(device)
	last := -1
	for _, a := range got {
		r := riskRank(a.Risk)
		if r < last {
			t.Errorf("risk ordering broken: %s (rank %d) appears after a higher-rank action", a.ID, r)
		}
		last = r
	}
}

// TestFindActionDef sanity-checks the dispatch table that
// ExecuteDeviceAction relies on. A typo in actions.go would otherwise
// surface as "unsupported action" at runtime.
func TestFindActionDef(t *testing.T) {
	for _, tt := range []string{"refresh", "firmware_check", "firmware_update", "reboot", "ble_pair", "wifi_scan", "eth_status", "factory_reset_wifi", "factory_reset"} {
		if findActionDef(tt) == nil {
			t.Errorf("action %q missing from catalog", tt)
		}
	}
	if findActionDef("definitely_not_an_action") != nil {
		t.Errorf("findActionDef returned non-nil for an unknown id")
	}
}
