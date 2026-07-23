package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"shellyadmin/internal/core/firmware"
	"shellyadmin/internal/core/setters"
	"shellyadmin/internal/models"
	"shellyadmin/internal/services/audit"
	"shellyadmin/internal/util"
)

type BulkActionRequest struct {
	Action  string   `json:"action"`
	MACs    []string `json:"macs"`
	Value   string   `json:"value"`
	Lat     float64  `json:"lat"`
	Lon     float64  `json:"lon"`
	Enabled *bool    `json:"enabled"`
	DryRun  bool     `json:"dry_run"`
}

type BulkActionPreview struct {
	Action   string             `json:"action"`
	Summary  string             `json:"summary"`
	Warnings []string           `json:"warnings"`
	Targets  []BulkActionTarget `json:"targets"`
}

type BulkActionTarget struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Name     string `json:"name"`
	Eligible bool   `json:"eligible"`
	Reason   string `json:"reason,omitempty"`
}

type BulkActionResult struct {
	MAC    string `json:"mac"`
	IP     string `json:"ip"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type DeviceCapability struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	State       string `json:"state"`
	Description string `json:"description,omitempty"`
}

type DeviceAction struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Description    string `json:"description"`
	Risk           string `json:"risk"`
	Supported      bool   `json:"supported"`
	RequiresOnline bool   `json:"requires_online"`
	Reason         string `json:"reason,omitempty"`
}

type DeviceDetail struct {
	Device       models.Device      `json:"device"`
	RawConfig    map[string]any     `json:"raw_config"`
	RawStatus    map[string]any     `json:"raw_status"`
	Capabilities []DeviceCapability `json:"capabilities"`
	Actions      []DeviceAction     `json:"actions"`
}

type DeviceActionRequest struct {
	Stage string `json:"stage"`
	// Instance is set by ExecuteDeviceAction when the requested action ID
	// has a `:N` suffix (per-component fan-out — see ADR-0010). Apply
	// functions for component-bound actions read this to know which
	// switch / cover / light / script to act on.
	Instance int `json:"instance,omitempty"`
}

type DeviceActionResult struct {
	Action string `json:"action"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Result any    `json:"result,omitempty"`
}

func (s *AppService) PreviewBulkAction(req BulkActionRequest) (BulkActionPreview, error) {
	req.Action = strings.TrimSpace(req.Action)
	index, err := s.deviceIndex()
	if err != nil {
		return BulkActionPreview{}, err
	}
	if err := validateBulkAction(req); err != nil {
		return BulkActionPreview{}, err
	}
	preview := BulkActionPreview{
		Action:   req.Action,
		Summary:  bulkActionSummary(req),
		Warnings: bulkActionWarnings(req),
		Targets:  make([]BulkActionTarget, 0, len(req.MACs)),
	}
	for _, mac := range req.MACs {
		device, ok := index[mac]
		if !ok {
			preview.Targets = append(preview.Targets, BulkActionTarget{MAC: mac, Eligible: false, Reason: "device not found"})
			continue
		}
		target := BulkActionTarget{
			MAC:      device.MAC,
			IP:       device.IP,
			Name:     util.FirstNonEmpty(device.Name, device.Serial, device.MAC),
			Eligible: true,
		}
		if !device.Online {
			target.Eligible = false
			target.Reason = "device currently offline"
		}
		if device.AuthRequired {
			target.Eligible = false
			target.Reason = util.FirstNonEmpty(device.AuthError, "device requires authentication")
		}
		preview.Targets = append(preview.Targets, target)
	}
	return preview, nil
}

func (s *AppService) BulkAction(ctx context.Context, req BulkActionRequest) ([]BulkActionResult, error) {
	req.Action = strings.TrimSpace(req.Action)
	if err := validateBulkAction(req); err != nil {
		return nil, err
	}
	index, err := s.deviceIndex()
	if err != nil {
		return nil, err
	}
	settings, err := s.db.GetSettings()
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(settings.ScanTimeout * float64(time.Second))
	results := make([]BulkActionResult, 0, len(req.MACs))
	for _, mac := range req.MACs {
		device, ok := index[mac]
		if !ok {
			results = append(results, BulkActionResult{MAC: mac, Status: "missing", Detail: "device not found"})
			continue
		}
		if !device.Online {
			results = append(results, BulkActionResult{MAC: device.MAC, IP: device.IP, Status: "skipped", Detail: "device offline"})
			continue
		}
		if device.AuthRequired {
			results = append(results, BulkActionResult{MAC: device.MAC, IP: device.IP, Status: "skipped", Detail: util.FirstNonEmpty(device.AuthError, "device requires authentication")})
			continue
		}
		success, detail := applyBulkAction(ctx, req, device, s.setterOptions(device, timeout))
		status := "failed"
		if success {
			status = "ok"
		}
		if success && req.Action == "set_auto_update" {
			device.FWAutoUpdate = strings.ToLower(strings.TrimSpace(req.Value))
			if uerr := s.db.UpsertDevice(device); uerr != nil {
				s.LogCtx(ctx, "warn", fmt.Sprintf("bulk set_auto_update persist mac=%s err=%v", device.MAC, uerr))
			}
		}
		results = append(results, BulkActionResult{MAC: device.MAC, IP: device.IP, Status: status, Detail: detail})
	}
	s.LogCtx(ctx, "INFO", fmt.Sprintf("bulk action applied action=%s targets=%d %s", req.Action, len(results), summarizeBulkResults(results)))
	return results, nil
}

func (s *AppService) GetDeviceDetail(target string) (DeviceDetail, error) {
	devices, err := s.GetDevices()
	if err != nil {
		return DeviceDetail{}, err
	}
	for _, device := range devices {
		if device.MAC != target && device.IP != target && device.Name != target {
			continue
		}
		return DeviceDetail{
			Device:       device,
			RawConfig:    parseRawMap(device.RawConfig),
			RawStatus:    parseRawMap(device.RawStatus),
			Capabilities: describeCapabilities(device),
			Actions:      describeDeviceActions(device),
		}, nil
	}
	return DeviceDetail{}, errors.New("device not found")
}

func (s *AppService) ListDeviceActions(target string) ([]DeviceAction, error) {
	detail, err := s.GetDeviceDetail(target)
	if err != nil {
		return nil, err
	}
	return detail.Actions, nil
}

func (s *AppService) ExecuteDeviceAction(ctx context.Context, target, action string, req DeviceActionRequest) (DeviceActionResult, error) {
	detail, err := s.GetDeviceDetail(target)
	if err != nil {
		return DeviceActionResult{}, err
	}
	action = strings.TrimSpace(action)
	// `detail.Actions` is already filtered through methodsCovered + the
	// online/auth gate. Verify the requested action (with any :N suffix
	// preserved, since fan-out IDs like "switch_toggle:0" are what gets
	// listed) is still in that set.
	if !supportedAction(detail.Actions, action) {
		return DeviceActionResult{}, fmt.Errorf("unsupported action: %s", action)
	}
	// For component-bound actions, peel the `:N` suffix and pass the
	// instance through DeviceActionRequest so Apply functions don't need
	// to re-parse it.
	baseID, instance := parseInstancedActionID(action)
	if instance >= 0 {
		req.Instance = instance
	}
	def := findActionDef(baseID)
	if def == nil {
		return DeviceActionResult{}, fmt.Errorf("unsupported action: %s", action)
	}
	// Thread the catalog risk through to the audit sink so audit_log
	// rows for action execution carry a structured risk_level alongside
	// the free-text message body. ADR-0010 carve-out: lets compliance
	// queries SELECT high-risk events directly.
	ctx = audit.WithRisk(ctx, def.risk)
	return def.apply(ctx, s, detail.Device, req)
}

func supportedAction(actions []DeviceAction, id string) bool {
	for _, action := range actions {
		if action.ID == id && action.Supported {
			return true
		}
	}
	return false
}

func describeCapabilities(device models.Device) []DeviceCapability {
	capabilities := []DeviceCapability{
		{ID: "generation", Label: "Generation", State: fmt.Sprintf("Gen %d", device.Gen)},
		{ID: "firmware", Label: "Firmware", State: util.FirstNonEmpty(device.FW, "unknown")},
		{ID: "compliance", Label: "Compliance", State: ternary(device.Compliant, "compliant", "issues")},
		{ID: "mqtt", Label: "MQTT", State: boolState(device.MQTTEnabled)},
		{ID: "cloud", Label: "Cloud", State: ternary(device.CloudConnected, "connected", "off")},
		{ID: "websocket", Label: "WebSocket", State: wsState(device)},
	}
	if device.MatterEnabled != nil {
		capabilities = append(capabilities, DeviceCapability{ID: "matter", Label: "Matter", State: boolState(device.MatterEnabled)})
	}
	if device.BLEGWEnabled != nil {
		capabilities = append(capabilities, DeviceCapability{ID: "ble_gateway", Label: "BLE Gateway", State: boolState(device.BLEGWEnabled)})
	}
	return capabilities
}

// describeDeviceActions is the catalog-driven action surface added in
// v0.1.8. The legacy hand-rolled slice is gone; the source of truth is
// internal/services/actions.go's `actionCatalog`, filtered against each
// device's SupportedMethods cache.
func describeDeviceActions(device models.Device) []DeviceAction {
	return describeAvailableActions(device)
}

func validateBulkAction(req BulkActionRequest) error {
	if len(req.MACs) == 0 {
		return errors.New("at least one device is required")
	}
	switch req.Action {
	case "set_location":
		if req.Lat < -90 || req.Lat > 90 || req.Lon < -180 || req.Lon > 180 {
			return errors.New("location requires valid latitude/longitude")
		}
	case "set_timezone", "set_mqtt_server", "set_sntp_server":
		if strings.TrimSpace(req.Value) == "" {
			return fmt.Errorf("%s requires value", req.Action)
		}
	case "set_mqtt_enabled", "set_cloud_enabled", "set_ble_enabled":
		if req.Enabled == nil {
			return fmt.Errorf("%s requires enabled", req.Action)
		}
	case "set_auto_update":
		switch strings.ToLower(strings.TrimSpace(req.Value)) {
		case firmware.AutoUpdateOff, firmware.AutoUpdateStable, firmware.AutoUpdateBeta:
		default:
			return fmt.Errorf("set_auto_update requires value one of: off, stable, beta")
		}
	case "reboot":
		// no extra fields required
	default:
		return fmt.Errorf("unsupported action: %s", req.Action)
	}
	return nil
}

func applyBulkAction(ctx context.Context, req BulkActionRequest, device models.Device, opts setters.Options) (bool, string) {
	st := setters.New(opts)
	switch req.Action {
	case "set_location":
		ok := st.SetLocation(ctx, device.IP, req.Lat, req.Lon)
		return ok, fmt.Sprintf("set location to %.5f, %.5f", req.Lat, req.Lon)
	case "set_timezone":
		ok := st.SetTimezone(ctx, device.IP, req.Value)
		return ok, fmt.Sprintf("set timezone to %s", req.Value)
	case "set_mqtt_server":
		ok := st.SetMQTTServer(ctx, device.IP, req.Value)
		return ok, fmt.Sprintf("set MQTT server to %s", req.Value)
	case "set_mqtt_enabled":
		ok := st.SetMQTTEnabled(ctx, device.IP, *req.Enabled)
		return ok, fmt.Sprintf("set MQTT %s", ternary(*req.Enabled, "enabled", "disabled"))
	case "set_sntp_server":
		ok := st.SetSNTPServer(ctx, device.IP, req.Value)
		return ok, fmt.Sprintf("set SNTP server to %s", req.Value)
	case "set_cloud_enabled":
		ok := st.SetCloudEnabled(ctx, device.IP, *req.Enabled)
		return ok, fmt.Sprintf("set Cloud %s", ternary(*req.Enabled, "enabled", "disabled"))
	case "set_ble_enabled":
		ok := st.SetBLEEnabled(ctx, device.IP, *req.Enabled)
		return ok, fmt.Sprintf("set BLE %s", ternary(*req.Enabled, "enabled", "disabled"))
	case "set_auto_update":
		mode := strings.ToLower(strings.TrimSpace(req.Value))
		fwOpts := firmware.Options{
			Timeout:       opts.Timeout,
			Scheme:        opts.Scheme,
			Username:      opts.Username,
			Password:      opts.Password,
			HA1:           opts.HA1,
			AllowInsecure: opts.AllowInsecure,
		}
		if err := firmware.SetAutoUpdate(ctx, device.IP, device.Gen, fwOpts, mode); err != nil {
			return false, fmt.Sprintf("auto-update %s: %v", mode, err)
		}
		return true, fmt.Sprintf("auto-update set to %s", mode)
	case "reboot":
		ok := st.Reboot(ctx, device.IP)
		return ok, "rebooted"
	default:
		return false, "unsupported action"
	}
}

func bulkActionSummary(req BulkActionRequest) string {
	switch req.Action {
	case "set_location":
		return fmt.Sprintf("Apply latitude %.5f and longitude %.5f to the selected devices.", req.Lat, req.Lon)
	case "set_timezone":
		return fmt.Sprintf("Set timezone to %s on the selected devices.", req.Value)
	case "set_mqtt_server":
		return fmt.Sprintf("Set MQTT server to %s on the selected devices.", req.Value)
	case "set_mqtt_enabled":
		return fmt.Sprintf("Set MQTT %s on the selected devices.", ternary(*req.Enabled, "enabled", "disabled"))
	case "set_sntp_server":
		return fmt.Sprintf("Set SNTP server to %s on the selected devices.", req.Value)
	case "set_cloud_enabled":
		return fmt.Sprintf("Set Cloud %s on the selected devices.", ternary(*req.Enabled, "enabled", "disabled"))
	case "set_ble_enabled":
		return fmt.Sprintf("Set BLE %s on the selected devices.", ternary(*req.Enabled, "enabled", "disabled"))
	case "set_auto_update":
		return fmt.Sprintf("Set firmware auto-update to %s on the selected devices.", strings.ToLower(strings.TrimSpace(req.Value)))
	case "reboot":
		return "Reboot the selected devices."
	default:
		return "Apply a bulk action to the selected devices."
	}
}

func bulkActionWarnings(req BulkActionRequest) []string {
	switch req.Action {
	case "set_location", "set_timezone", "set_mqtt_server", "set_mqtt_enabled", "set_sntp_server", "set_cloud_enabled", "set_ble_enabled":
		return []string{"Changes are sent directly to the devices and should be followed by a refresh to confirm the final state."}
	case "reboot":
		return []string{"Devices will be unreachable for ~20s; active scan/refresh jobs may error."}
	default:
		return nil
	}
}

func (s *AppService) deviceIndex() (map[string]models.Device, error) {
	devices, err := s.db.ListDevices()
	if err != nil {
		return nil, err
	}
	index := make(map[string]models.Device, len(devices))
	for _, device := range devices {
		index[device.MAC] = device
	}
	return index, nil
}

func parseRawMap(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{"_raw": raw}
	}
	return out
}

func boolState(value *bool) string {
	if value == nil {
		return "n/a"
	}
	return ternary(*value, "enabled", "disabled")
}

func wsState(device models.Device) string {
	if device.Gen <= 1 {
		return "unsupported"
	}
	return ternary(device.WSConnected, "connected", boolState(device.WSEnabled))
}

func ternary[T any](condition bool, yes, no T) T {
	if condition {
		return yes
	}
	return no
}

func SortedBulkActions() []string {
	actions := []string{"set_ble_enabled", "set_cloud_enabled", "set_location", "set_mqtt_enabled", "set_mqtt_server", "set_sntp_server", "set_timezone"}
	slices.Sort(actions)
	return actions
}

// bulkLogMaxMACs caps per-bucket MAC lists in audit log lines so a large bulk
// action does not produce an unreadable multi-KB log entry. Overflow is
// reported as "+N" so the reader knows the list was truncated.
const bulkLogMaxMACs = 20

// summarizeBulkResults renders a compact audit summary for a bulk action:
// per-status counts plus truncated MAC lists for ok and non-ok outcomes.
// Keeps detail text out so SanitizeLogMessage has nothing new to redact.
func summarizeBulkResults(results []BulkActionResult) string {
	var okCount, failedCount, skippedCount, missingCount int
	var okMACs, failedMACs, skippedMACs []string
	for _, result := range results {
		switch result.Status {
		case "ok":
			okCount++
			okMACs = append(okMACs, result.MAC)
		case "failed":
			failedCount++
			failedMACs = append(failedMACs, result.MAC)
		case "skipped":
			skippedCount++
			skippedMACs = append(skippedMACs, result.MAC)
		case "missing":
			missingCount++
		}
	}
	parts := []string{fmt.Sprintf("ok=%d", okCount), fmt.Sprintf("failed=%d", failedCount), fmt.Sprintf("skipped=%d", skippedCount), fmt.Sprintf("missing=%d", missingCount)}
	if list := truncateMACs(okMACs, bulkLogMaxMACs); list != "" {
		parts = append(parts, fmt.Sprintf("ok_macs=%s", list))
	}
	if list := truncateMACs(failedMACs, bulkLogMaxMACs); list != "" {
		parts = append(parts, fmt.Sprintf("failed_macs=%s", list))
	}
	if list := truncateMACs(skippedMACs, bulkLogMaxMACs); list != "" {
		parts = append(parts, fmt.Sprintf("skipped_macs=%s", list))
	}
	return strings.Join(parts, " ")
}

func truncateMACs(macs []string, max int) string {
	if len(macs) == 0 {
		return ""
	}
	if len(macs) <= max {
		return strings.Join(macs, ",")
	}
	return fmt.Sprintf("%s,+%d", strings.Join(macs[:max], ","), len(macs)-max)
}
