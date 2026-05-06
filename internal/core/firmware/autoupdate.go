package firmware

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"shellyadmin/internal/core/shellyclient"
)

// Auto-update modes that ShellyAdmin tracks. The Shelly device firmware does
// not expose a dedicated "auto update" RPC method; instead, the device's
// local web UI implements the toggle by registering a Schedule.* job that
// calls Shelly.Update on a recurring timer with origin="shelly_service".
const (
	AutoUpdateOff    = "off"
	AutoUpdateStable = "stable"
	AutoUpdateBeta   = "beta"

	// autoUpdateOrigin is the marker the device's local web UI writes onto
	// the schedule entry it creates for auto-update. We honour the same
	// marker so we don't clobber user-created Schedule jobs that happen to
	// also call Shelly.Update.
	autoUpdateOrigin = "shelly_service"

	// dailyMidnightTimespec is the cron-style timespec the device's own UI
	// writes when the user enables auto-update. Format: "s m h dom mon dow".
	dailyMidnightTimespec = "0 0 0 * * 0,1,2,3,4,5,6"
)

// ReadAutoUpdate inspects the device's Schedule.List response and returns
// the current auto-update mode: "off", "stable", or "beta". Any error
// (network, RPC) bubbles up; callers may persist the previous value when
// they get an error.
func ReadAutoUpdate(ctx context.Context, ip string, gen int, opts Options) (string, error) {
	if gen < 2 {
		return "", errors.New("gen1 devices not supported")
	}
	client := shellyclient.New(opts.toClientOptions())
	payload, err := client.RPC(ctx, ip, "Schedule.List", nil)
	if err != nil {
		return "", err
	}
	jobs, _ := payload["jobs"].([]any)
	for _, raw := range jobs {
		job, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		enabled, _ := job["enable"].(bool)
		if !enabled {
			continue
		}
		calls, _ := job["calls"].([]any)
		for _, callRaw := range calls {
			call, ok := callRaw.(map[string]any)
			if !ok {
				continue
			}
			method := strings.ToLower(stringValue(call["method"]))
			origin := stringValue(call["origin"])
			if method != "shelly.update" || origin != autoUpdateOrigin {
				continue
			}
			params, _ := call["params"].(map[string]any)
			stage := strings.ToLower(stringValue(params["stage"]))
			switch stage {
			case AutoUpdateStable:
				return AutoUpdateStable, nil
			case AutoUpdateBeta:
				return AutoUpdateBeta, nil
			}
		}
	}
	return AutoUpdateOff, nil
}

// SetAutoUpdate aligns the device's Schedule.* state with the desired mode
// ("off", "stable", or "beta"). It first deletes every existing
// origin="shelly_service" Shelly.Update job (idempotent), then creates a new
// one if the desired mode is not "off".
func SetAutoUpdate(ctx context.Context, ip string, gen int, opts Options, mode string) error {
	if gen < 2 {
		return errors.New("gen1 devices not supported")
	}
	client := shellyclient.New(opts.toClientOptions())
	return SetAutoUpdateOnClient(ctx, client, ip, mode)
}

// SetAutoUpdateOnClient is the same as SetAutoUpdate but reuses an existing
// shellyclient.Client. Used by the provisioner so a single template apply
// shares one auth-state with the rest of the section handlers.
func SetAutoUpdateOnClient(ctx context.Context, client *shellyclient.Client, ip, mode string) error {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = AutoUpdateOff
	}
	if mode != AutoUpdateOff && mode != AutoUpdateStable && mode != AutoUpdateBeta {
		return fmt.Errorf("invalid auto-update mode %q", mode)
	}

	listPayload, err := client.RPC(ctx, ip, "Schedule.List", nil)
	if err != nil {
		return err
	}
	jobs, _ := listPayload["jobs"].([]any)
	for _, raw := range jobs {
		job, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		calls, _ := job["calls"].([]any)
		isAutoUpdate := false
		for _, callRaw := range calls {
			call, ok := callRaw.(map[string]any)
			if !ok {
				continue
			}
			if strings.ToLower(stringValue(call["method"])) == "shelly.update" &&
				stringValue(call["origin"]) == autoUpdateOrigin {
				isAutoUpdate = true
				break
			}
		}
		if !isAutoUpdate {
			continue
		}
		idVal := job["id"]
		if _, derr := client.RPC(ctx, ip, "Schedule.Delete", map[string]any{"id": idVal}); derr != nil {
			return fmt.Errorf("delete schedule %v: %w", idVal, derr)
		}
	}

	if mode == AutoUpdateOff {
		return nil
	}

	_, err = client.RPC(ctx, ip, "Schedule.Create", map[string]any{
		"enable":   true,
		"timespec": dailyMidnightTimespec,
		"calls": []any{map[string]any{
			"method": "Shelly.Update",
			"params": map[string]any{"stage": mode},
			"origin": autoUpdateOrigin,
		}},
	})
	return err
}
