package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
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
	// component, when non-empty, makes this catalog entry per-instance:
	// describeAvailableActions expands one entry into N actions, one per
	// `<component>:N` key it finds in the device's RawStatus. Action IDs
	// gain a `:N` suffix; ExecuteDeviceAction parses that off and passes
	// the integer through DeviceActionRequest.Instance.
	component string
	apply     func(ctx context.Context, s *AppService, device models.Device, req DeviceActionRequest) (DeviceActionResult, error)
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
			if result.Batch != "" {
				updated.Batch = result.Batch
			}
			if result.FWID != "" {
				updated.FWID = result.FWID
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
		id:              "switch_toggle",
		label:           "Toggle",
		description:     "Toggle the switch on/off.",
		risk:            "medium",
		requiresOnline:  true,
		requiredMethods: []string{"Switch.Toggle"},
		component:       "switch",
		apply: func(ctx context.Context, s *AppService, d models.Device, req DeviceActionRequest) (DeviceActionResult, error) {
			ok, message := setters.New(s.setterOptions(d, 5*time.Second)).SwitchToggle(ctx, d.IP, req.Instance)
			if !ok {
				return DeviceActionResult{Action: fmt.Sprintf("switch_toggle:%d", req.Instance), Status: "failed", Detail: message}, nil
			}
			s.LogCtx(ctx, "INFO", fmt.Sprintf("device action switch_toggle target=%s id=%d", d.MAC, req.Instance))
			return DeviceActionResult{Action: fmt.Sprintf("switch_toggle:%d", req.Instance), Status: "ok", Detail: message}, nil
		},
	},
	{
		id:              "light_toggle",
		label:           "Toggle",
		description:     "Toggle the light on/off.",
		risk:            "low",
		requiresOnline:  true,
		requiredMethods: []string{"Light.Toggle"},
		component:       "light",
		apply: func(ctx context.Context, s *AppService, d models.Device, req DeviceActionRequest) (DeviceActionResult, error) {
			ok, message := setters.New(s.setterOptions(d, 5*time.Second)).LightToggle(ctx, d.IP, req.Instance)
			if !ok {
				return DeviceActionResult{Action: fmt.Sprintf("light_toggle:%d", req.Instance), Status: "failed", Detail: message}, nil
			}
			s.LogCtx(ctx, "INFO", fmt.Sprintf("device action light_toggle target=%s id=%d", d.MAC, req.Instance))
			return DeviceActionResult{Action: fmt.Sprintf("light_toggle:%d", req.Instance), Status: "ok", Detail: message}, nil
		},
	},
	{
		id:              "cover_open",
		label:           "Open",
		description:     "Drive the cover toward the fully-open position.",
		risk:            "medium",
		requiresOnline:  true,
		requiredMethods: []string{"Cover.Open"},
		component:       "cover",
		apply: func(ctx context.Context, s *AppService, d models.Device, req DeviceActionRequest) (DeviceActionResult, error) {
			ok, message := setters.New(s.setterOptions(d, 5*time.Second)).CoverOpen(ctx, d.IP, req.Instance)
			if !ok {
				return DeviceActionResult{Action: fmt.Sprintf("cover_open:%d", req.Instance), Status: "failed", Detail: message}, nil
			}
			s.LogCtx(ctx, "INFO", fmt.Sprintf("device action cover_open target=%s id=%d", d.MAC, req.Instance))
			return DeviceActionResult{Action: fmt.Sprintf("cover_open:%d", req.Instance), Status: "ok", Detail: message}, nil
		},
	},
	{
		id:              "cover_close",
		label:           "Close",
		description:     "Drive the cover toward the fully-closed position.",
		risk:            "medium",
		requiresOnline:  true,
		requiredMethods: []string{"Cover.Close"},
		component:       "cover",
		apply: func(ctx context.Context, s *AppService, d models.Device, req DeviceActionRequest) (DeviceActionResult, error) {
			ok, message := setters.New(s.setterOptions(d, 5*time.Second)).CoverClose(ctx, d.IP, req.Instance)
			if !ok {
				return DeviceActionResult{Action: fmt.Sprintf("cover_close:%d", req.Instance), Status: "failed", Detail: message}, nil
			}
			s.LogCtx(ctx, "INFO", fmt.Sprintf("device action cover_close target=%s id=%d", d.MAC, req.Instance))
			return DeviceActionResult{Action: fmt.Sprintf("cover_close:%d", req.Instance), Status: "ok", Detail: message}, nil
		},
	},
	{
		id:              "cover_stop",
		label:           "Stop",
		description:     "Halt the cover at its current position.",
		risk:            "low",
		requiresOnline:  true,
		requiredMethods: []string{"Cover.Stop"},
		component:       "cover",
		apply: func(ctx context.Context, s *AppService, d models.Device, req DeviceActionRequest) (DeviceActionResult, error) {
			ok, message := setters.New(s.setterOptions(d, 5*time.Second)).CoverStop(ctx, d.IP, req.Instance)
			if !ok {
				return DeviceActionResult{Action: fmt.Sprintf("cover_stop:%d", req.Instance), Status: "failed", Detail: message}, nil
			}
			s.LogCtx(ctx, "INFO", fmt.Sprintf("device action cover_stop target=%s id=%d", d.MAC, req.Instance))
			return DeviceActionResult{Action: fmt.Sprintf("cover_stop:%d", req.Instance), Status: "ok", Detail: message}, nil
		},
	},
	{
		id:              "ota_revert",
		label:           "Roll Back Firmware",
		description:     "Restore the previously-installed firmware. Unrecoverable from the app side once committed; use only when a recent update introduced a regression.",
		risk:            "high",
		requiresOnline:  true,
		requiredMethods: []string{"OTA.Revert"},
		apply: func(ctx context.Context, s *AppService, d models.Device, _ DeviceActionRequest) (DeviceActionResult, error) {
			ok, message := setters.New(s.setterOptions(d, 5*time.Second)).OTARevert(ctx, d.IP)
			if !ok {
				return DeviceActionResult{Action: "ota_revert", Status: "failed", Detail: message}, nil
			}
			s.LogCtx(ctx, "WARN", fmt.Sprintf("device action ota_revert target=%s", d.MAC))
			return DeviceActionResult{Action: "ota_revert", Status: "ok", Detail: message}, nil
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
// Catalog entries with a non-empty `component` are expanded into one
// DeviceAction per `<component>:N` instance the device exposes via
// RawStatus. The action ID gets a `:N` suffix; the label gets a "Switch 0"
// / "Cover 1" prefix so the front-end can group naturally.
//
// Risk grouping happens at the end; the catalog itself is already roughly
// risk-ordered.
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
		if def.component == "" {
			out = append(out, DeviceAction{
				ID:             def.id,
				Label:          def.label,
				Description:    def.description,
				Risk:           def.risk,
				Supported:      isSupported,
				RequiresOnline: def.requiresOnline,
				Reason:         reason,
			})
			continue
		}
		// Component-bound action: fan out per instance.
		instances := componentInstances(device, def.component)
		for _, inst := range instances {
			label := fmt.Sprintf("%s %d — %s", strings.Title(def.component), inst, def.label) //nolint:staticcheck // Title is fine here
			out = append(out, DeviceAction{
				ID:             fmt.Sprintf("%s:%d", def.id, inst),
				Label:          label,
				Description:    def.description,
				Risk:           def.risk,
				Supported:      isSupported,
				RequiresOnline: def.requiresOnline,
				Reason:         reason,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return riskRank(out[i].Risk) < riskRank(out[j].Risk)
	})
	return out
}

// componentInstanceRE matches the standard Shelly `<type>:<id>` key shape
// in RawStatus / RawConfig — captures the integer id.
var componentInstanceRE = regexp.MustCompile(`^[a-z0-9_]+:(\d+)$`)

// componentInstances returns the sorted list of integer ids the device
// exposes for the given component type (e.g. "switch" → [0,1,2,3] for a
// Pro 4 PM). Reads RawStatus first; falls back to empty if RawStatus is
// missing or unparseable. Non-contiguous ids are preserved.
func componentInstances(device models.Device, componentType string) []int {
	if device.RawStatus == "" {
		return nil
	}
	var status map[string]json.RawMessage
	if err := json.Unmarshal([]byte(device.RawStatus), &status); err != nil {
		return nil
	}
	prefix := componentType + ":"
	var ids []int
	for key := range status {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		match := componentInstanceRE.FindStringSubmatch(key)
		if match == nil {
			continue
		}
		n, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		ids = append(ids, n)
	}
	sort.Ints(ids)
	return ids
}

// sysAltVariants extracts the alternative-firmware variants a device
// advertises under Shelly.GetStatus → sys.alt (firmware 2.0.0+): a Zigbee or
// Matter build for the same hardware. Read from the cached RawStatus — no
// extra RPC, same source as componentInstances. Returns nil when the device
// exposes none (the common case). Variants are sorted by id for stable output.
func sysAltVariants(device models.Device) []models.AltFirmwareVariant {
	altMap, ok := sysSubObject(device.RawStatus, "alt")
	if !ok {
		return nil
	}
	out := make([]models.AltFirmwareVariant, 0, len(altMap))
	for id, raw := range altMap {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		v := models.AltFirmwareVariant{
			ID:   id,
			Name: stringField(entry["name"]),
			Desc: stringField(entry["desc"]),
		}
		if stable, ok := entry["stable"].(map[string]any); ok {
			v.Stable = stringField(stable["version"])
		}
		if beta, ok := entry["beta"].(map[string]any); ok {
			v.Beta = stringField(beta["version"])
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// sysProvisioning returns the Shelly.GetStatus → sys.provisioning object
// (secure-provisioning state, firmware 2.0.0+) as a raw map, or nil when the
// device is not enrolled (fleet-wide default). Read-only passthrough.
func sysProvisioning(device models.Device) map[string]any {
	obj, ok := sysSubObject(device.RawStatus, "provisioning")
	if !ok {
		return nil
	}
	return obj
}

// sysSubObject pulls RawStatus → sys → <key> as a map. Shared by the alt /
// provisioning derivers; both live under the device's sys component status.
func sysSubObject(rawStatus, key string) (map[string]any, bool) {
	if rawStatus == "" {
		return nil, false
	}
	var status map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawStatus), &status); err != nil {
		return nil, false
	}
	sysRaw, ok := status["sys"]
	if !ok {
		return nil, false
	}
	var sys map[string]any
	if err := json.Unmarshal(sysRaw, &sys); err != nil {
		return nil, false
	}
	obj, ok := sys[key].(map[string]any)
	if !ok || len(obj) == 0 {
		return nil, false
	}
	return obj, true
}

func stringField(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// parseInstancedActionID splits a "switch_toggle:0" form into ("switch_toggle", 0).
// Returns the original id and -1 when there's no `:N` suffix — used by
// ExecuteDeviceAction to dispatch component-bound actions back to their
// catalog entry while passing the instance through DeviceActionRequest.
func parseInstancedActionID(id string) (string, int) {
	colon := strings.LastIndex(id, ":")
	if colon < 0 || colon == len(id)-1 {
		return id, -1
	}
	suffix := id[colon+1:]
	n, err := strconv.Atoi(suffix)
	if err != nil {
		return id, -1
	}
	return id[:colon], n
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
