package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"shellyadmin/internal/core/compliance"
	"shellyadmin/internal/db"
	"shellyadmin/internal/models"
	"shellyadmin/internal/services"
)

// register wires every read-only tool onto the given server. The audit
// helper logs each call (or error) through svc.LogCtx so MCP activity
// shows up in /api/logs filterable by request_id.
func register(server *mcp.Server, svc *services.AppService) {
	registerDeviceTools(server, svc)
	registerJobStatusTools(server, svc)
	registerTemplateTools(server, svc)
	registerCredentialTool(server, svc)
	registerSettingsTool(server, svc)
	registerLogsTool(server, svc)
	registerComplianceTool(server, svc)
	registerActionTools(server, svc)
}

// tool wraps a typed handler with the standard audit-logging boilerplate
// every read-only tool needs.
func tool[In, Out any](svc *services.AppService, name string, fn func(context.Context, In) (Out, error)) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		out, err := fn(ctx, in)
		if err != nil {
			svc.LogCtx(ctx, "warn", fmt.Sprintf("mcp tool error: %s err=%v", name, err))
			var zero Out
			return nil, zero, err
		}
		svc.LogCtx(ctx, "info", fmt.Sprintf("mcp tool call: %s", name))
		return nil, out, nil
	}
}

// ----- list_devices / get_device / list_device_actions -----

type ListDevicesInput struct {
	Search string `json:"search,omitempty" jsonschema:"substring matched against name, MAC, IP, app, or model (case-insensitive)"`
	Gen    int    `json:"gen,omitempty" jsonschema:"filter by device generation (2, 3, 4); 0 = all"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max devices returned; 0 = unlimited"`
}

type ListDevicesOutput struct {
	Devices []models.Device `json:"devices"`
	Total   int             `json:"total"`
}

func filterDevices(in []models.Device, q ListDevicesInput) []models.Device {
	needle := strings.ToLower(strings.TrimSpace(q.Search))
	out := make([]models.Device, 0, len(in))
	for _, d := range in {
		if q.Gen != 0 && d.Gen != q.Gen {
			continue
		}
		if needle != "" {
			hay := strings.ToLower(d.Name + " " + d.MAC + " " + d.IP + " " + d.App + " " + d.Model)
			if !strings.Contains(hay, needle) {
				continue
			}
		}
		out = append(out, d)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out
}

type GetDeviceInput struct {
	Target string `json:"target" jsonschema:"MAC, IP, or device name"`
}

type ListDeviceActionsInput struct {
	Target string `json:"target" jsonschema:"MAC, IP, or device name"`
}

type ListDeviceActionsOutput struct {
	Actions []services.DeviceAction `json:"actions"`
}

func registerDeviceTools(server *mcp.Server, svc *services.AppService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_devices",
		Description: "List Shelly devices known to ShellyAdmin. Optional search/gen/limit filters.",
	}, tool(svc, "list_devices", func(_ context.Context, in ListDevicesInput) (ListDevicesOutput, error) {
		devices, err := svc.GetDevices()
		if err != nil {
			return ListDevicesOutput{}, err
		}
		filtered := filterDevices(devices, in)
		return ListDevicesOutput{Devices: filtered, Total: len(filtered)}, nil
	}))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_device",
		Description: "Fetch full detail (config, status, capabilities, available actions) for a single device.",
	}, tool(svc, "get_device", func(_ context.Context, in GetDeviceInput) (services.DeviceDetail, error) {
		return svc.GetDeviceDetail(strings.TrimSpace(in.Target))
	}))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_device_actions",
		Description: "List the actions (reboot, factory_reset, ota_revert, switch toggles, etc.) supported by a specific device.",
	}, tool(svc, "list_device_actions", func(_ context.Context, in ListDeviceActionsInput) (ListDeviceActionsOutput, error) {
		actions, err := svc.ListDeviceActions(strings.TrimSpace(in.Target))
		if err != nil {
			return ListDeviceActionsOutput{}, err
		}
		return ListDeviceActionsOutput{Actions: actions}, nil
	}))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "export_device",
		Description: "Export a device's full state (device row + raw config + raw status + capabilities) as a single JSON document.",
	}, tool(svc, "export_device", func(_ context.Context, in GetDeviceInput) (services.DeviceExport, error) {
		return svc.ExportDevice(strings.TrimSpace(in.Target))
	}))
}

// ----- job status tools (no input) -----

type emptyInput struct{}

// ScanPendingItem is the slim per-device summary returned by scan_status.
// We deliberately do NOT echo the full models.Device shape that
// services.ScanStatus carries — for a typical fleet that's >60 KB and
// trips MCP client per-tool output caps. Keep this list to identifying
// fields; callers needing full detail should call get_device after
// confirming a discovery.
type ScanPendingItem struct {
	MAC   string `json:"mac"`
	IP    string `json:"ip"`
	Name  string `json:"name"`
	Model string `json:"model"`
	Gen   int    `json:"gen"`
	App   string `json:"app"`
}

type ScanStatusOutput struct {
	Running bool              `json:"running"`
	Found   int               `json:"found"`
	Total   int               `json:"total"`
	Done    int               `json:"done"`
	Pending []ScanPendingItem `json:"pending"`
}

func slimScanPending(in []map[string]any) []ScanPendingItem {
	out := make([]ScanPendingItem, 0, len(in))
	for _, m := range in {
		// Round-trip through JSON so number/string coercion follows the
		// same rules as the SPA. Each map is a serialized models.Device.
		blob, err := json.Marshal(m)
		if err != nil {
			continue
		}
		var item ScanPendingItem
		if err := json.Unmarshal(blob, &item); err != nil {
			continue
		}
		out = append(out, item)
	}
	return out
}

func registerJobStatusTools(server *mcp.Server, svc *services.AppService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "scan_status",
		Description: "Status of the current/last network scan job (running flag, progress, pending discoveries). Pending entries are slim summaries — call get_device for full state.",
	}, tool(svc, "scan_status", func(_ context.Context, _ emptyInput) (ScanStatusOutput, error) {
		raw, err := svc.ScanStatus()
		if err != nil {
			return ScanStatusOutput{}, err
		}
		return ScanStatusOutput{
			Running: raw.Running,
			Found:   raw.Found,
			Total:   raw.Total,
			Done:    raw.Done,
			Pending: slimScanPending(raw.Pending),
		}, nil
	}))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "firmware_status",
		Description: "Status of the current/last firmware-check job (running flag, progress, per-device results).",
	}, tool(svc, "firmware_status", func(_ context.Context, _ emptyInput) (services.FirmwareStatus, error) {
		return svc.FirmwareStatus()
	}))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "firmware_install_status",
		Description: "Status of the current/last firmware-install job (running flag, progress, per-device install outcomes).",
	}, tool(svc, "firmware_install_status", func(_ context.Context, _ emptyInput) (services.FirmwareInstallStatus, error) {
		return svc.FirmwareInstallStatus()
	}))
}

// ----- templates -----

type ListTemplatesOutput struct {
	Names []string `json:"names"`
}

type GetTemplateInput struct {
	Name string `json:"name" jsonschema:"template name as stored under /api/templates"`
}

func registerTemplateTools(server *mcp.Server, svc *services.AppService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_templates",
		Description: "List provisioning template names.",
	}, tool(svc, "list_templates", func(_ context.Context, _ emptyInput) (ListTemplatesOutput, error) {
		names, err := svc.ListTemplates()
		if err != nil {
			return ListTemplatesOutput{}, err
		}
		return ListTemplatesOutput{Names: names}, nil
	}))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_template",
		Description: "Fetch a provisioning template by name (returns the raw JSON content and its credential reference, if any).",
	}, tool(svc, "get_template", func(_ context.Context, in GetTemplateInput) (services.TemplateRecord, error) {
		return svc.GetTemplate(strings.TrimSpace(in.Name))
	}))
}

// ----- credentials (redacted) -----

type ListCredentialsOutput struct {
	Credentials []RedactedCredential `json:"credentials"`
}

func registerCredentialTool(server *mcp.Server, svc *services.AppService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_credentials",
		Description: "List credential entries by name. Plaintext password and HA1 hashes are NEVER returned over MCP.",
	}, tool(svc, "list_credentials", func(_ context.Context, _ emptyInput) (ListCredentialsOutput, error) {
		creds, err := svc.ListCredentials()
		if err != nil {
			return ListCredentialsOutput{}, err
		}
		return ListCredentialsOutput{Credentials: redactCredentials(creds)}, nil
	}))
}

// ----- settings -----

// AppSettings as defined in internal/models/settings.go contains no
// secret material today (subnets, timeouts, badge classes, compliance
// rules). If a future field stores a token or hash, add a redactor here
// before exposing it.
func registerSettingsTool(server *mcp.Server, svc *services.AppService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_settings",
		Description: "Fetch the application settings (subnets, scan/refresh timeouts, compliance rules, firmware-check cadence, badge styling).",
	}, tool(svc, "get_settings", func(_ context.Context, _ emptyInput) (models.AppSettings, error) {
		return svc.GetSettings()
	}))
}

// ----- logs -----

type GetLogsInput struct {
	Level  string `json:"level,omitempty" jsonschema:"filter by exact level: INFO, WARN, ERROR (case-insensitive)"`
	Search string `json:"search,omitempty" jsonschema:"substring match against the log message (case-insensitive)"`
	Risk   string `json:"risk,omitempty" jsonschema:"filter by risk level (e.g. low, medium, high) — only present on action audits since v0.1.10"`
	Limit  int    `json:"limit,omitempty" jsonschema:"cap the number of rows returned; 0 = unlimited"`
}

type GetLogsOutput struct {
	Logs  []db.LogEntry `json:"logs"`
	Total int           `json:"total"`
}

func registerLogsTool(server *mcp.Server, svc *services.AppService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_logs",
		Description: "Fetch ShellyAdmin's audit log (the same rows the SPA's Logs page shows). Filterable by level, risk, and message substring.",
	}, tool(svc, "get_logs", func(_ context.Context, in GetLogsInput) (GetLogsOutput, error) {
		entries, err := svc.GetLogsFiltered(strings.TrimSpace(in.Level), strings.TrimSpace(in.Search), strings.TrimSpace(in.Risk))
		if err != nil {
			return GetLogsOutput{}, err
		}
		if in.Limit > 0 && len(entries) > in.Limit {
			entries = entries[:in.Limit]
		}
		return GetLogsOutput{Logs: entries, Total: len(entries)}, nil
	}))
}

// ----- compliance summary -----

type ComplianceSummaryOutput struct {
	TotalDevices        int            `json:"total_devices"`
	Compliant           int            `json:"compliant"`
	NonCompliant        int            `json:"non_compliant"`
	IssueCounts         map[string]int `json:"issue_counts"`
	NonCompliantDevices []DeviceIssue  `json:"non_compliant_devices"`
}

type DeviceIssue struct {
	MAC    string   `json:"mac"`
	Name   string   `json:"name"`
	IP     string   `json:"ip"`
	Issues []string `json:"issues"`
}

func registerComplianceTool(server *mcp.Server, svc *services.AppService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "compliance_summary",
		Description: "Run the configured compliance rules across the fleet and report which devices fail and why.",
	}, tool(svc, "compliance_summary", func(_ context.Context, _ emptyInput) (ComplianceSummaryOutput, error) {
		settings, err := svc.GetSettings()
		if err != nil {
			return ComplianceSummaryOutput{}, err
		}
		devices, err := svc.GetDevices()
		if err != nil {
			return ComplianceSummaryOutput{}, err
		}
		out := ComplianceSummaryOutput{
			TotalDevices:        len(devices),
			IssueCounts:         map[string]int{},
			NonCompliantDevices: []DeviceIssue{},
		}
		for _, d := range devices {
			ok, issues := compliance.Evaluate(d, settings.Compliance)
			if ok {
				out.Compliant++
				continue
			}
			out.NonCompliant++
			for _, issue := range issues {
				out.IssueCounts[issue]++
			}
			out.NonCompliantDevices = append(out.NonCompliantDevices, DeviceIssue{
				MAC:    d.MAC,
				Name:   d.Name,
				IP:     d.IP,
				Issues: append([]string(nil), issues...),
			})
		}
		return out, nil
	}))
}
