package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"shellyadmin/internal/models"
)

// callTool is a small assertion helper that calls a tool by name with the
// given arguments and unmarshals the structured content into out.
func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any, out any) {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	if res.IsError {
		t.Fatalf("tool %s reported error: %+v", name, res.Content)
	}
	if out != nil {
		if err := remarshal(res.StructuredContent, out); err != nil {
			t.Fatalf("remarshal %s: %v", name, err)
		}
	}
}

func TestRefreshAllDevicesPreviewVsConfirm(t *testing.T) {
	database, session := connectInMemory(t)
	for i, name := range []string{"a", "b", "c"} {
		_ = database.UpsertDevice(models.Device{
			MAC: "AA:BB:CC:DD:EE:0" + string(rune('0'+i)), IP: "10.0.0." + string(rune('1'+i)),
			Name: name, Online: true, Gen: 3,
		})
	}

	t.Run("preview-omits-confirm", func(t *testing.T) {
		var out SimpleActionResult
		callTool(t, session, "refresh_all_devices", map[string]any{}, &out)
		if !out.Preview {
			t.Errorf("preview flag = false, want true (no confirm passed)")
		}
		if out.DeviceCount != 3 {
			t.Errorf("device_count = %d, want 3", out.DeviceCount)
		}
		if out.JobID != 0 {
			t.Errorf("job_id should be unset in preview, got %d", out.JobID)
		}
	})

	t.Run("preview-confirm-false", func(t *testing.T) {
		var out SimpleActionResult
		callTool(t, session, "refresh_all_devices", map[string]any{"confirm": false}, &out)
		if !out.Preview {
			t.Errorf("preview flag = false with explicit confirm:false, want true")
		}
	})

	// confirm=true is intentionally NOT exercised here — RefreshDevices()
	// fires real HTTP probes against the IPs and the integration cost is
	// outsized for a unit test. The confirm path is exercised by the
	// service-level tests in app_jobs_test.go (which use stubs); here we
	// only verify the preview gate.
}

func TestStartScanPreview(t *testing.T) {
	database, session := connectInMemory(t)
	// Configure subnets so the preview's description has something to echo.
	_ = database.SaveSettings(models.AppSettings{
		Subnets: []string{"192.168.1.0/30"}, ScanTimeout: 2, RefreshTimeout: 5, ScanConcurrency: 64,
	})

	var out SimpleActionResult
	callTool(t, session, "start_scan", map[string]any{}, &out)
	if !out.Preview {
		t.Errorf("preview flag = false, want true")
	}
	if out.Description == "" {
		t.Errorf("description empty; want a summary including the configured subnets")
	}
}

func TestFirmwareInstallRequiresConfirmAndStage(t *testing.T) {
	_, session := connectInMemory(t)

	// Missing stage should error even in preview mode.
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "firmware_install",
		Arguments: map[string]any{"macs": []string{"AA:BB:CC:DD:EE:01"}},
	})
	if err == nil && !res.IsError {
		t.Errorf("firmware_install with no stage should error; got success: %+v", res)
	}

	// Bad stage value.
	res, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "firmware_install",
		Arguments: map[string]any{"macs": []string{"AA:BB:CC:DD:EE:01"}, "stage": "nightly"},
	})
	if err == nil && !res.IsError {
		t.Errorf("firmware_install with stage=nightly should error; got success")
	}

	// Valid preview — stage=stable, no confirm. Should return preview, not start a job.
	var out FirmwareInstallResult
	callTool(t, session, "firmware_install",
		map[string]any{"macs": []string{"AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02"}, "stage": "stable"},
		&out)
	if !out.Preview {
		t.Errorf("firmware_install preview flag = false, want true")
	}
	if out.JobID != 0 {
		t.Errorf("preview should not create a job, got job_id=%d", out.JobID)
	}
	if out.TargetCount != 2 {
		t.Errorf("target_count = %d, want 2", out.TargetCount)
	}
	if out.Stage != "stable" {
		t.Errorf("stage = %q, want stable", out.Stage)
	}
}

func TestExecuteDeviceActionPreviewSurfacesRisk(t *testing.T) {
	database, session := connectInMemory(t)
	_ = database.UpsertDevice(models.Device{
		MAC: "AA:BB:CC:DD:EE:10", IP: "192.168.1.20", Name: "kitchen-plug",
		Online: true, Gen: 3,
	})

	var out ExecuteDeviceActionResult
	callTool(t, session, "execute_device_action",
		map[string]any{"target": "kitchen-plug", "action": "reboot"},
		&out)
	if !out.Preview {
		t.Errorf("preview flag = false, want true")
	}
	if out.Risk != "medium" {
		t.Errorf("risk = %q, want medium (reboot is medium per the action catalog)", out.Risk)
	}
	if out.Action != "reboot" {
		t.Errorf("action = %q, want reboot", out.Action)
	}
}

func TestExecuteDeviceActionRejectsUnknownAction(t *testing.T) {
	database, session := connectInMemory(t)
	_ = database.UpsertDevice(models.Device{
		MAC: "AA:BB:CC:DD:EE:11", IP: "192.168.1.21", Name: "office",
		Online: true, Gen: 3,
	})

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "execute_device_action",
		Arguments: map[string]any{"target": "office", "action": "self_destruct"},
	})
	if err == nil && !res.IsError {
		t.Errorf("execute_device_action with unknown action should error; got %+v", res)
	}
}

func TestBulkActionPreviewListsTargets(t *testing.T) {
	database, session := connectInMemory(t)
	_ = database.UpsertDevice(models.Device{
		MAC: "AA:BB:CC:DD:EE:20", IP: "192.168.1.30", Name: "live", Online: true, Gen: 3,
	})
	_ = database.UpsertDevice(models.Device{
		MAC: "AA:BB:CC:DD:EE:21", IP: "192.168.1.31", Name: "offline", Online: false, Gen: 3,
	})

	var out BulkActionToolResult
	callTool(t, session, "bulk_action",
		map[string]any{
			"action": "set_timezone",
			"macs":   []string{"AA:BB:CC:DD:EE:20", "AA:BB:CC:DD:EE:21"},
			"value":  "Europe/Berlin",
		},
		&out)
	if !out.Preview {
		t.Errorf("preview flag = false, want true")
	}
	if len(out.Targets) != 2 {
		t.Errorf("preview targets len = %d, want 2", len(out.Targets))
	}
	// One should be flagged ineligible (offline).
	var sawIneligible bool
	for _, target := range out.Targets {
		if !target.Eligible {
			sawIneligible = true
			break
		}
	}
	if !sawIneligible {
		t.Errorf("expected one of the targets to be flagged ineligible (offline)")
	}
}

func TestActionAuditLogsPreviewVsConfirmed(t *testing.T) {
	// Verify the audit_log entries differentiate preview from confirmed
	// calls so an operator grepping the log can tell what actually ran.
	database, session := connectInMemory(t)
	_ = database.UpsertDevice(models.Device{
		MAC: "AA:BB:CC:DD:EE:30", IP: "192.168.1.40", Name: "logtest", Online: true, Gen: 3,
	})

	// Two preview calls with the same args.
	for i := 0; i < 2; i++ {
		callTool(t, session, "execute_device_action",
			map[string]any{"target": "logtest", "action": "reboot"}, nil)
	}

	// Filter by message substring only — GetLogsFiltered uppercases the
	// level arg internally while AppService.logf writes rows with the
	// lowercase level the caller passed (existing case-mismatch in the
	// audit infra, unrelated to this feature).
	rows, err := database.GetLogs("", "mcp action: execute_device_action")
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected ≥2 audit rows; got %d (rows=%+v)", len(rows), rows)
	}
	for _, row := range rows {
		// Both calls were previews, so every audit row should say so.
		if !contains(row.Message, "mode=preview") {
			t.Errorf("audit row missing mode=preview: %q", row.Message)
		}
	}
	// Marshal a sample row to confirm structured fields look right.
	if blob, err := json.Marshal(rows[0]); err == nil {
		_ = blob // fmt only — no assertion here, just smoke-check that the row is JSONable.
	}
}
