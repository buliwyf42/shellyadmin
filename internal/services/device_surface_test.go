package services

import (
	"fmt"
	"strings"
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

func TestSummarizeBulkResults_Empty(t *testing.T) {
	got := summarizeBulkResults(nil)
	want := "ok=0 failed=0 skipped=0 missing=0"
	if got != want {
		t.Fatalf("summarizeBulkResults(nil) = %q, want %q", got, want)
	}
}

func TestSummarizeBulkResults_AllOk(t *testing.T) {
	results := []BulkActionResult{
		{MAC: "AA:BB:CC:DD:EE:01", Status: "ok"},
		{MAC: "AA:BB:CC:DD:EE:02", Status: "ok"},
	}
	got := summarizeBulkResults(results)
	if !strings.Contains(got, "ok=2") {
		t.Errorf("missing ok count: %q", got)
	}
	if !strings.Contains(got, "ok_macs=AA:BB:CC:DD:EE:01,AA:BB:CC:DD:EE:02") {
		t.Errorf("missing or incorrect ok_macs list: %q", got)
	}
	if strings.Contains(got, "failed_macs=") || strings.Contains(got, "skipped_macs=") {
		t.Errorf("unexpected non-ok macs listed: %q", got)
	}
}

func TestSummarizeBulkResults_MixedStatuses(t *testing.T) {
	results := []BulkActionResult{
		{MAC: "AA:BB:CC:DD:EE:01", Status: "ok"},
		{MAC: "AA:BB:CC:DD:EE:02", Status: "failed"},
		{MAC: "AA:BB:CC:DD:EE:03", Status: "skipped"},
		{MAC: "AA:BB:CC:DD:EE:04", Status: "missing"},
	}
	got := summarizeBulkResults(results)
	for _, want := range []string{"ok=1", "failed=1", "skipped=1", "missing=1", "ok_macs=AA:BB:CC:DD:EE:01", "failed_macs=AA:BB:CC:DD:EE:02", "skipped_macs=AA:BB:CC:DD:EE:03"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q: %q", want, got)
		}
	}
}

func TestSummarizeBulkResults_OverflowTruncation(t *testing.T) {
	results := make([]BulkActionResult, 0, 25)
	for i := 0; i < 25; i++ {
		results = append(results, BulkActionResult{MAC: fmt.Sprintf("AA:BB:CC:DD:EE:%02d", i), Status: "ok"})
	}
	got := summarizeBulkResults(results)
	if !strings.Contains(got, "ok=25") {
		t.Errorf("missing ok=25: %q", got)
	}
	if !strings.Contains(got, "+5") {
		t.Errorf("expected '+5' overflow marker: %q", got)
	}
	if strings.Count(got, "AA:BB:CC:DD:EE:") != bulkLogMaxMACs {
		t.Errorf("expected exactly %d MACs listed, got count %d in %q", bulkLogMaxMACs, strings.Count(got, "AA:BB:CC:DD:EE:"), got)
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
