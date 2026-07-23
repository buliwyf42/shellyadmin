package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"shellyadmin/internal/services"
	"shellyadmin/internal/services/audit"
)

// confirmPolicy is appended verbatim to every state-changing tool's
// description so the operator-approval requirement is unmissable from
// the LLM's tool-listing surface.
const confirmPolicy = `

OPERATOR APPROVAL REQUIRED. This tool changes device state. The first call MUST omit confirm (or set it to false) to receive a structured preview describing what would happen. Surface the preview to the operator in plain language and obtain explicit yes/no approval. Only after the operator says yes may you call again with confirm=true to execute. Each preview/execute call is audit-logged with the request_id so the operator can verify what ran.`

// SimpleActionResult is the structured-content shape returned by the
// state-changing tools whose result is well-described by a small set of
// common fields. Tools with richer results (firmware_install, bulk_action,
// execute_device_action) define their own output types instead.
type SimpleActionResult struct {
	// Preview is true when the call ran in preview mode (confirm=false)
	// and no state change happened. Operators reading the audit log can
	// pair this with the request_id to confirm at-most-one execute call
	// per intent.
	Preview bool `json:"preview"`
	// Description is human-readable; the LLM is expected to surface this
	// (or a paraphrase) to the operator before passing confirm=true.
	Description string `json:"description"`
	// DeviceCount is set when the action targets a quantifiable set of
	// devices.
	DeviceCount int `json:"device_count,omitempty"`
	// Detail carries any additional message the AppService method
	// surfaced (e.g. "scan already running"). Optional.
	Detail string `json:"detail,omitempty"`
}

// actionTool is the registration helper for state-changing tools. It
// extracts the confirm flag, attaches the catalog risk level to the
// context (so audit rows carry risk_level), and emits a preview/action
// audit line before/after the underlying call.
func actionTool[In, Out any](
	svc *services.AppService,
	name, risk string,
	confirmedFn func(In) bool,
	fn func(context.Context, In) (Out, error),
) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		ctx = audit.WithRisk(ctx, risk)
		confirmed := confirmedFn(in)
		out, err := fn(ctx, in)
		mode := "preview"
		if confirmed {
			mode = "confirmed"
		}
		if err != nil {
			svc.LogCtx(ctx, "warn", fmt.Sprintf("mcp action error: %s mode=%s err=%v", name, mode, err))
			var zero Out
			return nil, zero, err
		}
		svc.LogCtx(ctx, "info", fmt.Sprintf("mcp action: %s mode=%s", name, mode))
		return nil, out, nil
	}
}

// registerActionTools wires the state-changing tools onto server. All
// tools follow the same confirm-or-preview policy described in
// confirmPolicy above.
func registerActionTools(server *mcp.Server, svc *services.AppService) {
	registerRefreshTools(server, svc)
	registerScanTools(server, svc)
	registerFirmwareActionTools(server, svc)
	registerDeviceActionTool(server, svc)
	registerBulkActionTool(server, svc)
}

// ---- refresh_device, refresh_all_devices ----

type RefreshDeviceInput struct {
	Target  string `json:"target" jsonschema:"MAC, IP, or device name"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"set to true to execute; without it, returns a preview only"`
}

type RefreshAllDevicesInput struct {
	Confirm bool `json:"confirm,omitempty" jsonschema:"set to true to execute; without it, returns a preview only"`
}

func registerRefreshTools(server *mcp.Server, svc *services.AppService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "refresh_device",
		Description: "Re-probe a single device (HTTP+RPC) and update its stored snapshot. Risk: low — read-only with respect to the device, only ShellyAdmin's local cache changes." + confirmPolicy,
	}, actionTool(svc, "refresh_device", "low",
		func(in RefreshDeviceInput) bool { return in.Confirm },
		func(ctx context.Context, in RefreshDeviceInput) (SimpleActionResult, error) {
			target := strings.TrimSpace(in.Target)
			if target == "" {
				return SimpleActionResult{}, fmt.Errorf("target required")
			}
			detail, err := svc.GetDeviceDetail(target)
			if err != nil {
				return SimpleActionResult{}, err
			}
			label := detail.Device.Name
			if label == "" {
				label = detail.Device.MAC
			}
			if !in.Confirm {
				return SimpleActionResult{
					Preview:     true,
					Description: fmt.Sprintf("would re-probe device %s (mac=%s ip=%s)", label, detail.Device.MAC, detail.Device.IP),
					DeviceCount: 1,
				}, nil
			}
			if _, err := svc.RefreshDevice(ctx, target); err != nil {
				return SimpleActionResult{}, err
			}
			return SimpleActionResult{
				Description: fmt.Sprintf("refreshed device %s (mac=%s)", label, detail.Device.MAC),
				DeviceCount: 1,
			}, nil
		}))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "refresh_all_devices",
		Description: "Re-probe every known device. Spawns a background refresh job. Risk: low — read-only with respect to devices." + confirmPolicy,
	}, actionTool(svc, "refresh_all_devices", "low",
		func(in RefreshAllDevicesInput) bool { return in.Confirm },
		func(ctx context.Context, in RefreshAllDevicesInput) (SimpleActionResult, error) {
			devices, err := svc.GetDevices()
			if err != nil {
				return SimpleActionResult{}, err
			}
			if !in.Confirm {
				return SimpleActionResult{
					Preview:     true,
					Description: fmt.Sprintf("would refresh all %d known devices", len(devices)),
					DeviceCount: len(devices),
				}, nil
			}
			if _, err := svc.RefreshDevices(ctx); err != nil {
				return SimpleActionResult{}, err
			}
			return SimpleActionResult{
				Description: fmt.Sprintf("refresh started for %d devices; poll scan_status / list_devices for progress", len(devices)),
				DeviceCount: len(devices),
			}, nil
		}))
}

// ---- start_scan, confirm_scan ----

type StartScanInput struct {
	Confirm bool `json:"confirm,omitempty" jsonschema:"set to true to execute; without it, returns a preview only"`
}

type ConfirmScanInput struct {
	MACs    []string `json:"macs,omitempty" jsonschema:"list of MAC addresses from scan_status.pending to register; empty array means register everything found"`
	Confirm bool     `json:"confirm,omitempty" jsonschema:"set to true to execute; without it, returns a preview only"`
}

func registerScanTools(server *mcp.Server, svc *services.AppService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_scan",
		Description: "Kick off a network scan over the configured subnets. Discovered devices land in scan_status.pending; nothing is registered until confirm_scan is called. Risk: low — read-only with respect to devices on the network." + confirmPolicy,
	}, actionTool(svc, "start_scan", "low",
		func(in StartScanInput) bool { return in.Confirm },
		func(ctx context.Context, in StartScanInput) (SimpleActionResult, error) {
			settings, err := svc.GetSettings()
			if err != nil {
				return SimpleActionResult{}, err
			}
			subnets := strings.Join(settings.Subnets, ", ")
			if !in.Confirm {
				return SimpleActionResult{
					Preview:     true,
					Description: fmt.Sprintf("would start a scan over subnets [%s] (mDNS=%v)", subnets, settings.EnableMDNS),
				}, nil
			}
			if err := svc.StartScan(); err != nil {
				return SimpleActionResult{}, err
			}
			return SimpleActionResult{
				Description: fmt.Sprintf("scan started over subnets [%s]; poll scan_status for progress and pending discoveries", subnets),
			}, nil
		}))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "confirm_scan",
		Description: "Register devices found by the most recent scan into the persistent inventory. Pass macs=[] to register everything pending; otherwise pass a subset. Risk: medium — adds rows to the device store and starts ongoing refresh / firmware probes against them." + confirmPolicy,
	}, actionTool(svc, "confirm_scan", "medium",
		func(in ConfirmScanInput) bool { return in.Confirm },
		func(ctx context.Context, in ConfirmScanInput) (SimpleActionResult, error) {
			if !in.Confirm {
				count := len(in.MACs)
				suffix := "the specified subset"
				if count == 0 {
					suffix = "every pending discovery"
				}
				return SimpleActionResult{
					Preview:     true,
					Description: fmt.Sprintf("would register %s into the device inventory (count=%d when set; 0 means all)", suffix, count),
					DeviceCount: count,
				}, nil
			}
			registered, err := svc.ConfirmScan(in.MACs)
			if err != nil {
				return SimpleActionResult{}, err
			}
			return SimpleActionResult{
				Description: fmt.Sprintf("registered %d device(s) from the pending scan", registered),
				DeviceCount: registered,
			}, nil
		}))
}

// ---- firmware_check, firmware_install ----

type FirmwareCheckInput struct {
	Confirm bool `json:"confirm,omitempty" jsonschema:"set to true to execute; without it, returns a preview only"`
}

type FirmwareInstallInput struct {
	MACs    []string `json:"macs" jsonschema:"MAC addresses of the devices to update"`
	Stage   string   `json:"stage" jsonschema:"firmware channel: \"stable\" or \"beta\""`
	Confirm bool     `json:"confirm,omitempty" jsonschema:"set to true to execute; without it, returns a preview only"`
}

type FirmwareInstallResult struct {
	Preview     bool   `json:"preview"`
	Description string `json:"description"`
	JobID       int64  `json:"job_id,omitempty"`
	Stage       string `json:"stage"`
	TargetCount int    `json:"target_count"`
}

func registerFirmwareActionTools(server *mcp.Server, svc *services.AppService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "firmware_check",
		Description: "Query every known device's available firmware versions (stable + beta). Spawns a firmware_check job. Risk: low — read-only." + confirmPolicy,
	}, actionTool(svc, "firmware_check", "low",
		func(in FirmwareCheckInput) bool { return in.Confirm },
		func(ctx context.Context, in FirmwareCheckInput) (SimpleActionResult, error) {
			devices, err := svc.GetDevices()
			if err != nil {
				return SimpleActionResult{}, err
			}
			if !in.Confirm {
				return SimpleActionResult{
					Preview:     true,
					Description: fmt.Sprintf("would check firmware for %d devices", len(devices)),
					DeviceCount: len(devices),
				}, nil
			}
			total, err := svc.StartFirmwareCheck()
			if err != nil {
				return SimpleActionResult{}, err
			}
			return SimpleActionResult{
				Description: fmt.Sprintf("firmware check started for %d devices; poll firmware_status for results", total),
				DeviceCount: total,
			}, nil
		}))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "firmware_install",
		Description: "Trigger Shelly.Update on the named devices, pulling firmware from either the stable or beta channel. Devices reboot mid-install. Risk: HIGH — a bad firmware can require physical access to recover." + confirmPolicy,
	}, actionTool(svc, "firmware_install", "high",
		func(in FirmwareInstallInput) bool { return in.Confirm },
		func(ctx context.Context, in FirmwareInstallInput) (FirmwareInstallResult, error) {
			stage := strings.TrimSpace(strings.ToLower(in.Stage))
			if stage != "stable" && stage != "beta" {
				return FirmwareInstallResult{}, fmt.Errorf("stage must be \"stable\" or \"beta\"")
			}
			if len(in.MACs) == 0 {
				return FirmwareInstallResult{}, fmt.Errorf("macs required (at least one)")
			}
			if !in.Confirm {
				return FirmwareInstallResult{
					Preview:     true,
					Stage:       stage,
					TargetCount: len(in.MACs),
					Description: fmt.Sprintf("would trigger %s firmware install on %d device(s): %s", stage, len(in.MACs), strings.Join(in.MACs, ", ")),
				}, nil
			}
			jobID, count, err := svc.StartFirmwareInstall(in.MACs, stage)
			if err != nil {
				return FirmwareInstallResult{}, err
			}
			return FirmwareInstallResult{
				JobID:       jobID,
				Stage:       stage,
				TargetCount: count,
				Description: fmt.Sprintf("%s firmware install started for %d device(s); poll firmware_install_status for outcomes", stage, count),
			}, nil
		}))
}

// ---- execute_device_action ----

type ExecuteDeviceActionInput struct {
	Target  string `json:"target" jsonschema:"MAC, IP, or device name"`
	Action  string `json:"action" jsonschema:"action ID from list_device_actions, e.g. \"reboot\", \"factory_reset\", \"switch_toggle:0\", \"cover_open:0\""`
	Stage   string `json:"stage,omitempty" jsonschema:"firmware channel for firmware_update action: \"stable\" or \"beta\". Ignored for other actions."`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"set to true to execute; without it, returns a preview only"`
}

type ExecuteDeviceActionResult struct {
	Preview     bool   `json:"preview"`
	Description string `json:"description"`
	Target      string `json:"target,omitempty"`
	Action      string `json:"action,omitempty"`
	Risk        string `json:"risk,omitempty"`
	Status      string `json:"status,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

func registerDeviceActionTool(server *mcp.Server, svc *services.AppService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "execute_device_action",
		Description: "Run a per-device action (reboot, factory_reset, switch_toggle:N, cover_open:N, ota_revert, etc.). The exhaustive set of valid actions per device is what list_device_actions(target) returns; risk varies per action and is included in the preview. Risk: VARIES — see preview." + confirmPolicy,
	}, actionTool(svc, "execute_device_action", "high", // we tag every call as "high" for audit; preview surfaces the per-action risk separately
		func(in ExecuteDeviceActionInput) bool { return in.Confirm },
		func(ctx context.Context, in ExecuteDeviceActionInput) (ExecuteDeviceActionResult, error) {
			target := strings.TrimSpace(in.Target)
			actionID := strings.TrimSpace(in.Action)
			if target == "" || actionID == "" {
				return ExecuteDeviceActionResult{}, fmt.Errorf("target and action are required")
			}
			detail, err := svc.GetDeviceDetail(target)
			if err != nil {
				return ExecuteDeviceActionResult{}, err
			}
			// Resolve the action descriptor (label + risk + supported flag).
			var found *services.DeviceAction
			for i := range detail.Actions {
				if detail.Actions[i].ID == actionID {
					found = &detail.Actions[i]
					break
				}
			}
			if found == nil {
				return ExecuteDeviceActionResult{}, fmt.Errorf("unknown or unavailable action %q for device %s — see list_device_actions", actionID, target)
			}
			if !found.Supported {
				return ExecuteDeviceActionResult{}, fmt.Errorf("action %q is not supported on this device", actionID)
			}
			if !in.Confirm {
				return ExecuteDeviceActionResult{
					Preview:     true,
					Target:      detail.Device.MAC,
					Action:      actionID,
					Risk:        found.Risk,
					Description: fmt.Sprintf("would run %s (%s, risk=%s) on device %s (mac=%s)", found.Label, actionID, found.Risk, detail.Device.Name, detail.Device.MAC),
				}, nil
			}
			req := services.DeviceActionRequest{Stage: strings.TrimSpace(in.Stage)}
			result, err := svc.ExecuteDeviceAction(ctx, target, actionID, req)
			if err != nil {
				return ExecuteDeviceActionResult{}, err
			}
			return ExecuteDeviceActionResult{
				Target:      detail.Device.MAC,
				Action:      actionID,
				Risk:        found.Risk,
				Status:      result.Status,
				Detail:      result.Detail,
				Description: fmt.Sprintf("ran %s on device %s: status=%s", actionID, detail.Device.Name, result.Status),
			}, nil
		}))
}

// ---- bulk_action ----

type BulkActionInput struct {
	Action  string   `json:"action" jsonschema:"bulk action ID, e.g. \"set_timezone\", \"set_sntp_server\", \"set_mqtt_server\", \"reboot\", \"set_auto_update\""`
	MACs    []string `json:"macs" jsonschema:"MAC addresses to apply the action to"`
	Value   string   `json:"value,omitempty" jsonschema:"primary value for the action (timezone, hostname, etc.) — see ShellyAdmin's bulk-action API for what each action expects"`
	Confirm bool     `json:"confirm,omitempty" jsonschema:"set to true to execute; without it, returns a preview only"`
}

type BulkActionToolResult struct {
	Preview     bool                        `json:"preview"`
	Description string                      `json:"description"`
	Action      string                      `json:"action"`
	Targets     []services.BulkActionTarget `json:"targets,omitempty"` // populated in preview mode
	Results     []services.BulkActionResult `json:"results,omitempty"` // populated when confirmed
	Warnings    []string                    `json:"warnings,omitempty"`
}

func registerBulkActionTool(server *mcp.Server, svc *services.AppService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "bulk_action",
		Description: "Apply a setting change across many devices in one shot — set_timezone, set_sntp_server, set_mqtt_server, set_auto_update, reboot, etc. The preview lists which devices are eligible and which would be skipped (offline, locked, missing capability). Risk: HIGH for actions that mutate device config or trigger reboots." + confirmPolicy,
	}, actionTool(svc, "bulk_action", "high",
		func(in BulkActionInput) bool { return in.Confirm },
		func(ctx context.Context, in BulkActionInput) (BulkActionToolResult, error) {
			req := services.BulkActionRequest{
				Action: strings.TrimSpace(in.Action),
				MACs:   in.MACs,
				Value:  in.Value,
			}
			if req.Action == "" || len(req.MACs) == 0 {
				return BulkActionToolResult{}, fmt.Errorf("action and macs are required")
			}
			if !in.Confirm {
				preview, err := svc.PreviewBulkAction(req)
				if err != nil {
					return BulkActionToolResult{}, err
				}
				return BulkActionToolResult{
					Preview:     true,
					Action:      req.Action,
					Description: preview.Summary,
					Targets:     preview.Targets,
					Warnings:    preview.Warnings,
				}, nil
			}
			results, err := svc.BulkAction(ctx, req)
			if err != nil {
				return BulkActionToolResult{}, err
			}
			ok := 0
			for _, r := range results {
				if r.Status == "ok" {
					ok++
				}
			}
			return BulkActionToolResult{
				Action:      req.Action,
				Description: fmt.Sprintf("bulk %s applied to %d device(s); %d ok, %d skipped/failed", req.Action, len(results), ok, len(results)-ok),
				Results:     results,
			}, nil
		}))
}
