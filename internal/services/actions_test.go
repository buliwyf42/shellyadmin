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
// gets every fleet-wide catalog action. Component-bound entries
// (`switch_toggle`, `cover_open`, etc.) require RawStatus to fan out, so
// an unprobed device with no RawStatus correctly shows zero of those —
// exposing four "Switch — Toggle" buttons with no idea which switch
// they'd hit would be worse UX than just hiding them until probed.
func TestDescribeAvailableActions_FallbackUnprobed(t *testing.T) {
	device := models.Device{MAC: "AA", Online: true, Gen: 2}
	got := describeDeviceActions(device)

	expected := 0
	for _, def := range actionCatalog {
		if def.component == "" {
			expected++
		}
	}
	if len(got) != expected {
		t.Fatalf("unprobed device action count = %d, want %d (fleet-wide catalog entries)",
			len(got), expected)
	}
}

// TestDescribeAvailableActions_ComponentFanout proves the fan-out path:
// an unprobed device with two switches in RawStatus should produce two
// switch_toggle:N rows (one per instance), with stable per-instance IDs.
func TestDescribeAvailableActions_ComponentFanout(t *testing.T) {
	device := models.Device{
		MAC:       "AA",
		Online:    true,
		Gen:       2,
		RawStatus: `{"switch:0":{"output":true},"switch:1":{"output":false},"sys":{}}`,
	}
	got := describeDeviceActions(device)
	var found []string
	for _, a := range got {
		if a.ID == "switch_toggle:0" || a.ID == "switch_toggle:1" {
			found = append(found, a.ID)
		}
	}
	if len(found) != 2 {
		t.Errorf("expected 2 switch_toggle fan-out actions, got %d (%v)", len(found), found)
	}
}

// TestComponentInstances pins the JSON-key parser so a future RawStatus
// shape change can't silently break per-component fan-out.
func TestComponentInstances(t *testing.T) {
	device := models.Device{
		RawStatus: `{"switch:0":{},"switch:2":{},"cover:0":{},"sys":{},"switch:notanint":{}}`,
	}
	if got := componentInstances(device, "switch"); len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Errorf("componentInstances(switch) = %v, want [0 2] (skip non-integer ids, sort)", got)
	}
	if got := componentInstances(device, "cover"); len(got) != 1 || got[0] != 0 {
		t.Errorf("componentInstances(cover) = %v, want [0]", got)
	}
	if got := componentInstances(device, "light"); len(got) != 0 {
		t.Errorf("componentInstances(light) = %v, want empty", got)
	}
	if got := componentInstances(models.Device{}, "switch"); len(got) != 0 {
		t.Errorf("empty RawStatus should yield empty instance list, got %v", got)
	}
}

// TestParseInstancedActionID covers the dispatch path that turns
// "switch_toggle:1" back into ("switch_toggle", 1).
func TestParseInstancedActionID(t *testing.T) {
	tests := []struct {
		in       string
		wantBase string
		wantInst int
	}{
		{"refresh", "refresh", -1},
		{"firmware_update", "firmware_update", -1},
		{"switch_toggle:0", "switch_toggle", 0},
		{"cover_close:7", "cover_close", 7},
		{"trailing:colon:", "trailing:colon:", -1}, // empty suffix → no instance
		{"bad_id:notanint", "bad_id:notanint", -1},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			base, inst := parseInstancedActionID(tt.in)
			if base != tt.wantBase || inst != tt.wantInst {
				t.Errorf("parseInstancedActionID(%q) = (%q, %d), want (%q, %d)", tt.in, base, inst, tt.wantBase, tt.wantInst)
			}
		})
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
	for _, banned := range []string{"firmware_check", "firmware_update", "wifi_scan", "factory_reset", "ota_revert"} {
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
	for _, tt := range []string{
		"refresh", "firmware_check", "firmware_update", "reboot", "ble_pair",
		"wifi_scan", "eth_status",
		"switch_toggle", "light_toggle", "cover_open", "cover_close", "cover_stop",
		"ota_revert", "factory_reset_wifi", "factory_reset",
	} {
		if findActionDef(tt) == nil {
			t.Errorf("action %q missing from catalog", tt)
		}
	}
	if findActionDef("definitely_not_an_action") != nil {
		t.Errorf("findActionDef returned non-nil for an unknown id")
	}
}
