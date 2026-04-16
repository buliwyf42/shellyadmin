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
			Name:     firstNonEmpty(device.Name, device.Serial, device.MAC),
			Eligible: true,
		}
		if !device.Online {
			target.Eligible = false
			target.Reason = "device currently offline"
		}
		if device.AuthRequired {
			target.Eligible = false
			target.Reason = firstNonEmpty(device.AuthError, "device requires authentication")
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
			results = append(results, BulkActionResult{MAC: device.MAC, IP: device.IP, Status: "skipped", Detail: firstNonEmpty(device.AuthError, "device requires authentication")})
			continue
		}
		success, detail := applyBulkAction(ctx, req, device, timeout)
		status := "failed"
		if success {
			status = "ok"
		}
		results = append(results, BulkActionResult{MAC: device.MAC, IP: device.IP, Status: status, Detail: detail})
	}
	okCount := 0
	for _, result := range results {
		if result.Status == "ok" {
			okCount++
		}
	}
	s.Log("INFO", fmt.Sprintf("bulk action applied action=%s targets=%d ok=%d", req.Action, len(results), okCount))
	return results, nil
}

func (s *AppService) GetDeviceDetail(target string) (DeviceDetail, error) {
	devices, err := s.GetDevices()
	if err != nil {
		return DeviceDetail{}, err
	}
	for _, device := range devices {
		if device.MAC != target && device.IP != target {
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
	if !supportedAction(detail.Actions, action) {
		return DeviceActionResult{}, fmt.Errorf("unsupported action: %s", action)
	}
	switch action {
	case "refresh":
		if _, err := s.RefreshDevice(ctx, target); err != nil {
			return DeviceActionResult{}, err
		}
		s.Log("INFO", fmt.Sprintf("device action refresh target=%s", target))
		return DeviceActionResult{Action: action, Status: "ok", Detail: "device refreshed"}, nil
	case "firmware_check":
		stage := firstNonEmpty(req.Stage, "stable")
		result := firmware.CheckOne(ctx, detail.Device, stage, 5*time.Second)
		s.Log("INFO", fmt.Sprintf("device action firmware_check target=%s stage=%s status=%s", target, stage, result.Status))
		return DeviceActionResult{Action: action, Status: "ok", Detail: "firmware check completed", Result: result}, nil
	case "firmware_update":
		stage := firstNonEmpty(req.Stage, "stable")
		results, err := s.FirmwareUpdate(ctx, []string{detail.Device.MAC}, stage)
		if err != nil {
			return DeviceActionResult{}, err
		}
		s.Log("INFO", fmt.Sprintf("device action firmware_update target=%s stage=%s", target, stage))
		return DeviceActionResult{Action: action, Status: "ok", Detail: "firmware update triggered", Result: results}, nil
	case "reboot":
		timeout := 5 * time.Second
		if !setters.Reboot(ctx, detail.Device.IP, detail.Device.Gen, timeout) {
			return DeviceActionResult{Action: action, Status: "failed", Detail: "device did not accept reboot request"}, nil
		}
		s.Log("INFO", fmt.Sprintf("device action reboot target=%s", target))
		return DeviceActionResult{Action: action, Status: "ok", Detail: "reboot requested"}, nil
	default:
		return DeviceActionResult{}, fmt.Errorf("unsupported action: %s", action)
	}
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
		{ID: "firmware", Label: "Firmware", State: firstNonEmpty(device.FW, "unknown")},
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

func describeDeviceActions(device models.Device) []DeviceAction {
	unsupportedReason := ""
	if !device.Online {
		unsupportedReason = "device offline"
	} else if device.AuthRequired {
		unsupportedReason = firstNonEmpty(device.AuthError, "device requires authentication")
	}
	return []DeviceAction{
		{
			ID:             "refresh",
			Label:          "Refresh",
			Description:    "Re-read the device and update the stored snapshot.",
			Risk:           "low",
			Supported:      true,
			RequiresOnline: false,
		},
		{
			ID:             "firmware_check",
			Label:          "Firmware Check",
			Description:    "Check the selected firmware channel for this device only.",
			Risk:           "low",
			Supported:      device.Online && !device.AuthRequired,
			RequiresOnline: true,
			Reason:         unsupportedReason,
		},
		{
			ID:             "firmware_update",
			Label:          "Firmware Update",
			Description:    "Trigger a firmware update for this device.",
			Risk:           "high",
			Supported:      device.Online && !device.AuthRequired,
			RequiresOnline: true,
			Reason:         unsupportedReason,
		},
		{
			ID:             "reboot",
			Label:          "Reboot",
			Description:    "Request a device reboot over the local API.",
			Risk:           "medium",
			Supported:      device.Online && !device.AuthRequired,
			RequiresOnline: true,
			Reason:         unsupportedReason,
		},
	}
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
	case "set_mqtt_enabled":
		if req.Enabled == nil {
			return errors.New("set_mqtt_enabled requires enabled")
		}
	case "set_24h":
	default:
		return fmt.Errorf("unsupported action: %s", req.Action)
	}
	return nil
}

func applyBulkAction(ctx context.Context, req BulkActionRequest, device models.Device, timeout time.Duration) (bool, string) {
	switch req.Action {
	case "set_location":
		ok := setters.SetLocation(ctx, device.IP, req.Lat, req.Lon, device.Gen, timeout)
		return ok, fmt.Sprintf("set location to %.5f, %.5f", req.Lat, req.Lon)
	case "set_timezone":
		ok := setters.SetTimezone(ctx, device.IP, req.Value, device.Gen, timeout)
		return ok, fmt.Sprintf("set timezone to %s", req.Value)
	case "set_mqtt_server":
		ok := setters.SetMQTTServer(ctx, device.IP, req.Value, device.Gen, timeout)
		return ok, fmt.Sprintf("set MQTT server to %s", req.Value)
	case "set_mqtt_enabled":
		ok := setters.SetMQTTEnabled(ctx, device.IP, *req.Enabled, device.Gen, timeout)
		return ok, fmt.Sprintf("set MQTT %s", ternary(*req.Enabled, "enabled", "disabled"))
	case "set_sntp_server":
		ok := setters.SetSNTPServer(ctx, device.IP, req.Value, device.Gen, timeout)
		return ok, fmt.Sprintf("set SNTP server to %s", req.Value)
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
	default:
		return "Apply a bulk action to the selected devices."
	}
}

func bulkActionWarnings(req BulkActionRequest) []string {
	switch req.Action {
	case "set_location", "set_timezone", "set_mqtt_server", "set_mqtt_enabled", "set_sntp_server":
		return []string{"Changes are sent directly to the devices and should be followed by a refresh to confirm the final state."}
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func ternary[T any](condition bool, yes, no T) T {
	if condition {
		return yes
	}
	return no
}

func SortedBulkActions() []string {
	actions := []string{"set_24h", "set_location", "set_mqtt_enabled", "set_mqtt_server", "set_sntp_server", "set_timezone"}
	slices.Sort(actions)
	return actions
}
