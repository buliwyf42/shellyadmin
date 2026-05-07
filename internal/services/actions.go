package services

import (
	"context"
	"fmt"
	"sort"
	"time"

	"shellyadmin/internal/core/firmware"
	"shellyadmin/internal/core/setters"
	"shellyadmin/internal/models"
	"shellyadmin/internal/util"
)

// actionDef describes one action the per-device action surface can offer.
// The catalog (see actionCatalog below) is filtered against each device's
// SupportedMethods cache so unsupported actions are simply not rendered,
// rather than rendered with `Supported: false` or surfaced as runtime
// failures. See ADR-0010 for the underlying decision.
type actionDef struct {
	id              string
	label           string
	description     string
	risk            string
	requiresOnline  bool
	requiredMethods []string
	apply           func(ctx context.Context, s *AppService, device models.Device, req DeviceActionRequest) (DeviceActionResult, error)
}

// methodSet builds a fast lookup from a device's SupportedMethods cache.
// nil slice → empty set, but the catalog filter treats that case specially
// (see methodsCovered).
func methodSet(device models.Device) map[string]bool {
	out := make(map[string]bool, len(device.SupportedMethods))
	for _, m := range device.SupportedMethods {
		out[m] = true
	}
	return out
}

// methodsCovered returns true when the device supports every method the
// action requires. The "no methods cached yet" case (empty set + non-empty
// requiredMethods) is treated as "assume supported" so an upgraded fleet
// keeps working before the next firmware-check populates the cache.
// Required methods that are empty strings (or the slice itself is empty)
// always count as covered — used for refresh / local-only actions.
func methodsCovered(supported map[string]bool, required []string, probed bool) bool {
	if len(required) == 0 {
		return true
	}
	if !probed {
		return true
	}
	for _, m := range required {
		if !supported[m] {
			return false
		}
	}
	return true
}

// actionCatalog is the source of truth for what per-device actions exist.
// Each entry's apply() owns its own RPC plumbing, audit message, and result
// shaping; ExecuteDeviceAction dispatches by id and otherwise stays trivial.
//
// Adding an action: append a new actionDef with the methods it requires
// and an apply() that returns a DeviceActionResult. Per-component fan-out
// (switch:N, cover:N, ...) is intentionally deferred to a follow-up — see
// ADR-0010 "Rollout plan".
var actionCatalog = []actionDef{
	{
		id:             "refresh",
		label:          "Refresh",
		description:    "Re-read the device and update the stored snapshot.",
		risk:           "low",
		requiresOnline: false,
		// No required methods — refresh is local-only (re-probes, then
		// upserts). Always available.
		apply: func(ctx context.Context, s *AppService, d models.Device, _ DeviceActionRequest) (DeviceActionResult, error) {
			if _, err := s.RefreshDevice(ctx, d.MAC); err != nil {
				return DeviceActionResult{}, err
			}
			s.LogCtx(ctx, "INFO", fmt.Sprintf("device action refresh target=%s", d.MAC))
			return DeviceActionResult{Action: "refresh", Status: "ok", Detail: "device refreshed"}, nil
		},
	},
	{
		id:              "firmware_check",
		label:           "Firmware Check",
		description:     "Check the available firmware versions for this device.",
		risk:            "low",
		requiresOnline:  true,
		requiredMethods: []string{"Shelly.CheckForUpdate"},
		apply: func(ctx context.Context, s *AppService, d models.Device, _ DeviceActionRequest) (DeviceActionResult, error) {
			result := firmware.CheckOneWithOptions(ctx, d, s.firmwareOptions(d, 10*time.Second))
			updated := d
			if result.CurrentVer != "" {
				updated.FW = result.CurrentVer
			}
			updated.FWAvailableStable = result.StableVer
			updated.FWAvailableBeta = result.BetaVer
			updated.FWCheckedAt = result.CheckedAt
			if mode, autoErr := firmware.ReadAutoUpdate(ctx, updated.IP, updated.Gen, s.firmwareOptions(updated, 5*time.Second)); autoErr == nil {
				updated.FWAutoUpdate = mode
			}
			if methods, mErr := firmware.ListSupportedMethods(ctx, updated.IP, updated.Gen, s.firmwareOptions(updated, 5*time.Second)); mErr == nil {
				updated.SupportedMethods = methods
			}
			if uerr := s.db.UpsertDevice(updated); uerr != nil {
				s.LogCtx(ctx, "warn", fmt.Sprintf("device action firmware_check persist target=%s err=%v", d.MAC, uerr))
			}
			s.LogCtx(ctx, "INFO", fmt.Sprintf("device action firmware_check target=%s status=%s", d.MAC, result.Status))
			return DeviceActionResult{Action: "firmware_check", Status: "ok", Detail: "firmware check completed", Result: result}, nil
		},
	},
	{
		id:              "firmware_update",
		label:           "Firmware Update",
		description:     "Trigger a firmware update for this device.",
		risk:            "high",
		requiresOnline:  true,
		requiredMethods: []string{"Shelly.Update"},
		apply: func(ctx context.Context, s *AppService, d models.Device, req DeviceActionRequest) (DeviceActionResult, error) {
			stage := util.FirstNonEmpty(req.Stage, "stable")
			results, err := s.FirmwareUpdate(ctx, []string{d.MAC}, stage)
			if err != nil {
				return DeviceActionResult{}, err
			}
			s.LogCtx(ctx, "INFO", fmt.Sprintf("device action firmware_update target=%s stage=%s", d.MAC, stage))
			return DeviceActionResult{Action: "firmware_update", Status: "ok", Detail: "firmware update triggered", Result: results}, nil
		},
	},
	{
		id:              "reboot",
		label:           "Reboot",
		description:     "Restart the device.",
		risk:            "medium",
		requiresOnline:  true,
		requiredMethods: []string{"Shelly.Reboot"},
		apply: func(ctx context.Context, s *AppService, d models.Device, _ DeviceActionRequest) (DeviceActionResult, error) {
			if !setters.New(s.setterOptions(d, 5*time.Second)).Reboot(ctx, d.IP) {
				return DeviceActionResult{Action: "reboot", Status: "failed", Detail: "device did not accept reboot request"}, nil
			}
			s.LogCtx(ctx, "INFO", fmt.Sprintf("device action reboot target=%s", d.MAC))
			return DeviceActionResult{Action: "reboot", Status: "ok", Detail: "reboot requested"}, nil
		},
	},
	{
		id:              "ble_pair",
		label:           "BLE Pair",
		description:     "Put the device into BLE pairing mode (FW 2.0+).",
		risk:            "low",
		requiresOnline:  true,
		requiredMethods: []string{"BLE.Pair"},
		apply: func(ctx context.Context, s *AppService, d models.Device, _ DeviceActionRequest) (DeviceActionResult, error) {
			ok, supported, message := setters.New(s.setterOptions(d, 5*time.Second)).BLEPair(ctx, d.IP)
			if !supported {
				// Fallback for the rollout window where SupportedMethods
				// hasn't been probed yet — the catalog filter let us
				// through, but the device still 404s the RPC. Surface as
				// "skipped" rather than "failed" to match v0.1.6 behaviour.
				s.LogCtx(ctx, "INFO", fmt.Sprintf("device action ble_pair target=%s status=skipped (unsupported firmware)", d.MAC))
				return DeviceActionResult{Action: "ble_pair", Status: "skipped", Detail: message}, nil
			}
			if !ok {
				return DeviceActionResult{Action: "ble_pair", Status: "failed", Detail: message}, nil
			}
			s.LogCtx(ctx, "INFO", fmt.Sprintf("device action ble_pair target=%s", d.MAC))
			return DeviceActionResult{Action: "ble_pair", Status: "ok", Detail: message}, nil
		},
	},
	{
		id:              "wifi_scan",
		label:           "Wi-Fi Scan",
		description:     "Scan for visible Wi-Fi networks. Useful for diagnosing connectivity issues.",
		risk:            "low",
		requiresOnline:  true,
		requiredMethods: []string{"Wifi.Scan"},
		apply: func(ctx context.Context, s *AppService, d models.Device, _ DeviceActionRequest) (DeviceActionResult, error) {
			result, err := setters.New(s.setterOptions(d, 10*time.Second)).WiFiScan(ctx, d.IP)
			if err != nil {
				return DeviceActionResult{Action: "wifi_scan", Status: "failed", Detail: err.Error()}, nil
			}
			s.LogCtx(ctx, "INFO", fmt.Sprintf("device action wifi_scan target=%s", d.MAC))
			return DeviceActionResult{Action: "wifi_scan", Status: "ok", Detail: "wi-fi scan completed", Result: result}, nil
		},
	},
	{
		id:              "eth_status",
		label:           "Ethernet Status",
		description:     "Read live Ethernet link / IPv4 / IPv6 status.",
		risk:            "low",
		requiresOnline:  true,
		requiredMethods: []string{"Eth.GetStatus"},
		apply: func(ctx context.Context, s *AppService, d models.Device, _ DeviceActionRequest) (DeviceActionResult, error) {
			result, err := setters.New(s.setterOptions(d, 5*time.Second)).EthGetStatus(ctx, d.IP)
			if err != nil {
				return DeviceActionResult{Action: "eth_status", Status: "failed", Detail: err.Error()}, nil
			}
			s.LogCtx(ctx, "INFO", fmt.Sprintf("device action eth_status target=%s", d.MAC))
			return DeviceActionResult{Action: "eth_status", Status: "ok", Detail: "ethernet status read", Result: result}, nil
		},
	},
	{
		id:              "factory_reset_wifi",
		label:           "Reset Wi-Fi & Cloud",
		description:     "Clear stored Wi-Fi credentials and cloud config; the device returns to AP mode for re-provisioning. Scripts, KVS, and schedules are preserved.",
		risk:            "high",
		requiresOnline:  true,
		requiredMethods: []string{"Shelly.ResetWiFiConfig"},
		apply: func(ctx context.Context, s *AppService, d models.Device, _ DeviceActionRequest) (DeviceActionResult, error) {
			ok, message := setters.New(s.setterOptions(d, 5*time.Second)).ResetWiFiConfig(ctx, d.IP)
			if !ok {
				return DeviceActionResult{Action: "factory_reset_wifi", Status: "failed", Detail: message}, nil
			}
			s.LogCtx(ctx, "WARN", fmt.Sprintf("device action factory_reset_wifi target=%s", d.MAC))
			return DeviceActionResult{Action: "factory_reset_wifi", Status: "ok", Detail: message}, nil
		},
	},
	{
		id:              "factory_reset",
		label:           "Factory Reset",
		description:     "Wipe ALL persisted configuration on the device. Unrecoverable from the app side — the device must be re-provisioned afterward.",
		risk:            "high",
		requiresOnline:  true,
		requiredMethods: []string{"Shelly.FactoryReset"},
		apply: func(ctx context.Context, s *AppService, d models.Device, _ DeviceActionRequest) (DeviceActionResult, error) {
			ok, message := setters.New(s.setterOptions(d, 5*time.Second)).FactoryReset(ctx, d.IP)
			if !ok {
				return DeviceActionResult{Action: "factory_reset", Status: "failed", Detail: message}, nil
			}
			s.LogCtx(ctx, "WARN", fmt.Sprintf("device action factory_reset target=%s", d.MAC))
			return DeviceActionResult{Action: "factory_reset", Status: "ok", Detail: message}, nil
		},
	},
}

// describeAvailableActions builds the per-device DeviceAction list from the
// catalog. Actions whose required methods aren't in the device's
// SupportedMethods cache are omitted (or, when the cache is empty, kept as
// a fallback so the post-upgrade rollout window doesn't leave devices
// action-less).
//
// Risk grouping happens client-side; this layer returns a stable order so
// the audit log + tests can rely on it.
func describeAvailableActions(device models.Device) []DeviceAction {
	supported := methodSet(device)
	probed := len(supported) > 0
	unsupportedReason := unavailableReason(device)

	out := make([]DeviceAction, 0, len(actionCatalog))
	for _, def := range actionCatalog {
		if !methodsCovered(supported, def.requiredMethods, probed) {
			continue
		}
		isSupported := true
		reason := ""
		if def.requiresOnline && unsupportedReason != "" {
			isSupported = false
			reason = unsupportedReason
		}
		out = append(out, DeviceAction{
			ID:             def.id,
			Label:          def.label,
			Description:    def.description,
			Risk:           def.risk,
			Supported:      isSupported,
			RequiresOnline: def.requiresOnline,
			Reason:         reason,
		})
	}
	// Stable: catalog order is the source of truth, but we also sort by
	// risk inside the catalog already (low → medium → high) so the
	// front-end can render in returned order.
	sort.SliceStable(out, func(i, j int) bool {
		return riskRank(out[i].Risk) < riskRank(out[j].Risk)
	})
	return out
}

// unavailableReason mirrors the "device offline / auth required" guard that
// describeDeviceActions used to apply. Returns empty when the device is
// reachable; non-empty when an online-only action should be marked
// unsupported with that reason.
func unavailableReason(device models.Device) string {
	if !device.Online {
		return "device offline"
	}
	if device.AuthRequired {
		return util.FirstNonEmpty(device.AuthError, "device requires authentication")
	}
	return ""
}

// riskRank gives risk strings a stable ordering for the per-device action
// list. Unknown risks sort after the known ones.
func riskRank(risk string) int {
	switch risk {
	case "low":
		return 0
	case "medium":
		return 1
	case "high":
		return 2
	}
	return 3
}

// findActionDef returns the catalog entry matching the given action id, or
// nil if no entry exists. ExecuteDeviceAction uses this to dispatch instead
// of a giant switch.
func findActionDef(id string) *actionDef {
	for i := range actionCatalog {
		if actionCatalog[i].id == id {
			return &actionCatalog[i]
		}
	}
	return nil
}
