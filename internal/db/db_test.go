package db

import (
	"testing"

	"shellyadmin/internal/models"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	database, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestUpsertDevicesCommitsAllRowsAtomically(t *testing.T) {
	database := openTestDB(t)
	scanned := []models.Device{
		{MAC: "AA:BB:CC:DD:EE:01", IP: "192.168.1.10", Name: "alpha", Gen: 2},
		{MAC: "AA:BB:CC:DD:EE:02", IP: "192.168.1.11", Name: "beta", Gen: 2},
		{MAC: "AA:BB:CC:DD:EE:03", IP: "192.168.1.12", Name: "gamma", Gen: 2},
	}
	if err := database.UpsertDevices(scanned); err != nil {
		t.Fatalf("UpsertDevices: %v", err)
	}
	got, err := database.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("device count = %d, want 3", len(got))
	}
	for _, d := range got {
		if !d.Online {
			t.Fatalf("device %s should be marked online after scan", d.MAC)
		}
		if d.DeviceNum == 0 {
			t.Fatalf("device %s should have a non-zero DeviceNum", d.MAC)
		}
	}
}

func TestUpsertDevicesMarksMissingDevicesAfterTwoMisses(t *testing.T) {
	database := openTestDB(t)
	initial := []models.Device{
		{MAC: "AA:BB:CC:DD:EE:01", IP: "192.168.1.10", Name: "alpha", Gen: 2},
		{MAC: "AA:BB:CC:DD:EE:02", IP: "192.168.1.11", Name: "beta", Gen: 2},
	}
	if err := database.UpsertDevices(initial); err != nil {
		t.Fatalf("seed UpsertDevices: %v", err)
	}

	onlyAlpha := initial[:1]
	if err := database.UpsertDevices(onlyAlpha); err != nil {
		t.Fatalf("first miss UpsertDevices: %v", err)
	}
	if err := database.UpsertDevices(onlyAlpha); err != nil {
		t.Fatalf("second miss UpsertDevices: %v", err)
	}

	devices, err := database.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	for _, d := range devices {
		switch d.MAC {
		case "AA:BB:CC:DD:EE:01":
			if !d.Online {
				t.Fatalf("alpha should still be online")
			}
		case "AA:BB:CC:DD:EE:02":
			if d.Online {
				t.Fatalf("beta should be offline after 2 missed scans")
			}
			if d.ConsecutiveMisses < 2 {
				t.Fatalf("beta ConsecutiveMisses = %d, want >= 2", d.ConsecutiveMisses)
			}
		}
	}
}

func TestAddLogWithRequestIDRoundTrips(t *testing.T) {
	database := openTestDB(t)
	if err := database.AddLogWithRequestID("INFO", "scoped entry", "req-deadbeef"); err != nil {
		t.Fatalf("AddLogWithRequestID: %v", err)
	}
	if err := database.AddLog("INFO", "unscoped entry"); err != nil {
		t.Fatalf("AddLog: %v", err)
	}
	entries, err := database.GetLogs("", "")
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	// Newest first; scoped row was written second? Actually scoped row was
	// written first, so it is last.
	var scoped, unscoped *LogEntry
	for i := range entries {
		switch entries[i].Message {
		case "scoped entry":
			scoped = &entries[i]
		case "unscoped entry":
			unscoped = &entries[i]
		}
	}
	if scoped == nil || scoped.RequestID != "req-deadbeef" {
		t.Fatalf("scoped row request id = %+v, want req-deadbeef", scoped)
	}
	if unscoped == nil || unscoped.RequestID != "" {
		t.Fatalf("unscoped row should have empty request id, got %+v", unscoped)
	}
}

func TestUpsertDevicesReturnsErrorOnClosedDB(t *testing.T) {
	database, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err = database.UpsertDevices([]models.Device{
		{MAC: "AA:BB:CC:DD:EE:01", IP: "192.168.1.10", Gen: 2},
	})
	if err == nil {
		t.Fatal("UpsertDevices on closed DB returned nil error, want error")
	}
}

// A scan probe carries none of the firmware-cache fields nor the operator's
// TLS opt-out, so UpsertDevices must take them from the existing row. Before
// the fix a confirmed scan blanked all five fleet-wide — measured on a real
// fleet as fw_auto_update going from 44x "stable" to 43x empty.
func TestUpsertDevicesPreservesFirmwareCacheAndTLSOptOut(t *testing.T) {
	database := openTestDB(t)
	allowInsecure := true
	seeded := models.Device{
		MAC: "AA:BB:CC:DD:EE:01", IP: "192.168.1.10", Name: "alpha", Gen: 2,
		FWAvailableStable: "2.0.1", FWAvailableBeta: "2.0.2-beta1",
		FWCheckedAt: "2026-09-12T08:00:00Z", FWAutoUpdate: "stable",
		TLSAllowInsecure: &allowInsecure,
	}
	if err := database.UpsertDevices([]models.Device{seeded}); err != nil {
		t.Fatalf("seed UpsertDevices: %v", err)
	}

	// Second scan of the same device: the probe reports none of the five.
	rescanned := models.Device{MAC: seeded.MAC, IP: "192.168.1.10", Name: "alpha", Gen: 2}
	if err := database.UpsertDevices([]models.Device{rescanned}); err != nil {
		t.Fatalf("rescan UpsertDevices: %v", err)
	}

	got, err := database.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("device count = %d, want 1", len(got))
	}
	d := got[0]
	if d.FWAvailableStable != seeded.FWAvailableStable {
		t.Errorf("FWAvailableStable = %q, want %q", d.FWAvailableStable, seeded.FWAvailableStable)
	}
	if d.FWAvailableBeta != seeded.FWAvailableBeta {
		t.Errorf("FWAvailableBeta = %q, want %q", d.FWAvailableBeta, seeded.FWAvailableBeta)
	}
	if d.FWCheckedAt != seeded.FWCheckedAt {
		t.Errorf("FWCheckedAt = %q, want %q", d.FWCheckedAt, seeded.FWCheckedAt)
	}
	if d.FWAutoUpdate != seeded.FWAutoUpdate {
		t.Errorf("FWAutoUpdate = %q, want %q", d.FWAutoUpdate, seeded.FWAutoUpdate)
	}
	if d.TLSAllowInsecure == nil || !*d.TLSAllowInsecure {
		t.Errorf("TLSAllowInsecure = %v, want true", d.TLSAllowInsecure)
	}
}

// A device seen for the first time has nothing to carry forward: the probed
// zero values must land as-is rather than inheriting another row's cache.
func TestUpsertDevicesLeavesFirmwareCacheEmptyForNewDevice(t *testing.T) {
	database := openTestDB(t)
	if err := database.UpsertDevices([]models.Device{
		{MAC: "AA:BB:CC:DD:EE:01", IP: "192.168.1.10", Gen: 2, FWAutoUpdate: "stable", FWCheckedAt: "2026-09-12T08:00:00Z"},
	}); err != nil {
		t.Fatalf("seed UpsertDevices: %v", err)
	}
	if err := database.UpsertDevices([]models.Device{
		{MAC: "AA:BB:CC:DD:EE:01", IP: "192.168.1.10", Gen: 2},
		{MAC: "AA:BB:CC:DD:EE:02", IP: "192.168.1.11", Gen: 2},
	}); err != nil {
		t.Fatalf("second UpsertDevices: %v", err)
	}
	got, err := database.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	for _, d := range got {
		if d.MAC == "AA:BB:CC:DD:EE:02" && (d.FWAutoUpdate != "" || d.FWCheckedAt != "") {
			t.Errorf("new device inherited firmware cache: auto=%q checked=%q", d.FWAutoUpdate, d.FWCheckedAt)
		}
	}
}
