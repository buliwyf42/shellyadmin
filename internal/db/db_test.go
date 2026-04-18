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
