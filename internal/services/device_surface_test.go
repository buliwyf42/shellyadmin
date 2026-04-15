package services

import (
	"testing"

	"shellyadmin/internal/db"
	"shellyadmin/internal/models"
)

func TestPreviewBulkActionMarksOfflineAndAuthTargets(t *testing.T) {
	database, service := testService(t)
	_ = database.UpsertDevice(models.Device{MAC: "AA:BB:CC:DD:EE:01", IP: "192.168.1.10", Name: "online", Online: true, Gen: 2})
	_ = database.UpsertDevice(models.Device{MAC: "AA:BB:CC:DD:EE:02", IP: "192.168.1.11", Name: "offline", Online: false, Gen: 2})
	_ = database.UpsertDevice(models.Device{MAC: "AA:BB:CC:DD:EE:03", IP: "192.168.1.12", Name: "locked", Online: true, Gen: 2, AuthRequired: true, AuthError: "401 Unauthorized"})

	preview, err := service.PreviewBulkAction(BulkActionRequest{
		Action: "set_timezone",
		MACs:   []string{"AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02", "AA:BB:CC:DD:EE:03"},
		Value:  "Europe/Berlin",
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("PreviewBulkAction() error = %v", err)
	}
	if len(preview.Targets) != 3 {
		t.Fatalf("PreviewBulkAction() targets = %d, want 3", len(preview.Targets))
	}
	if !preview.Targets[0].Eligible {
		t.Fatalf("first target should be eligible")
	}
	if preview.Targets[1].Reason != "device currently offline" {
		t.Fatalf("offline target reason = %q", preview.Targets[1].Reason)
	}
	if preview.Targets[2].Reason != "401 Unauthorized" {
		t.Fatalf("auth target reason = %q", preview.Targets[2].Reason)
	}
}

func TestGetDeviceDetailIncludesSupportedActions(t *testing.T) {
	database, service := testService(t)
	rawConfig := `{"sys":{"location":{"tz":"Europe/Berlin"}}}`
	rawStatus := `{"cloud":{"connected":true}}`
	_ = database.UpsertDevice(models.Device{
		MAC:            "AA:BB:CC:DD:EE:04",
		IP:             "192.168.1.13",
		Name:           "detail-device",
		Online:         true,
		Gen:            2,
		FW:             "1.0.0",
		CloudConnected: true,
		RawConfig:      rawConfig,
		RawStatus:      rawStatus,
	})

	detail, err := service.GetDeviceDetail("AA:BB:CC:DD:EE:04")
	if err != nil {
		t.Fatalf("GetDeviceDetail() error = %v", err)
	}
	if detail.RawConfig["sys"] == nil {
		t.Fatalf("GetDeviceDetail() raw config missing expected content")
	}
	if len(detail.Actions) == 0 {
		t.Fatalf("GetDeviceDetail() actions should not be empty")
	}
	foundReboot := false
	for _, action := range detail.Actions {
		if action.ID == "reboot" {
			foundReboot = true
			if !action.Supported {
				t.Fatalf("reboot action should be supported")
			}
		}
	}
	if !foundReboot {
		t.Fatalf("reboot action not found")
	}
}

func TestValidateSettingsAllowsMDNSWithoutSubnets(t *testing.T) {
	err := ValidateSettings(models.AppSettings{
		EnableMDNS:      true,
		ScanTimeout:     2,
		RefreshTimeout:  5,
		ScanConcurrency: 64,
	})
	if err != nil {
		t.Fatalf("ValidateSettings() error = %v", err)
	}
}

func TestValidateSettingsRejectsEmptyScanTargets(t *testing.T) {
	err := ValidateSettings(models.AppSettings{
		EnableMDNS:      false,
		ScanTimeout:     2,
		RefreshTimeout:  5,
		ScanConcurrency: 64,
	})
	if err == nil {
		t.Fatal("ValidateSettings() error = nil, want error")
	}
	if got, want := err.Error(), "no scan targets configured; add at least one subnet in Settings or enable mDNS discovery"; got != want {
		t.Fatalf("ValidateSettings() error = %q, want %q", got, want)
	}
}

func testService(t *testing.T) (*db.DB, *AppService) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := NewAppService(database, t.TempDir(), func(level, msg string) {})
	return database, service
}
