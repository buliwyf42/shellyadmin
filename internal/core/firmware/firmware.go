package firmware

import (
	"context"
	"errors"
	"strings"
	"time"

	"shellyadmin/internal/core/shellyclient"
	"shellyadmin/internal/models"
)

// Result captures the outcome of a single Shelly.CheckForUpdate call. Both
// stable and beta versions are populated from the same response — the channel
// selector on the frontend is purely a display/install filter.
type Result struct {
	IP           string `json:"ip"`
	MAC          string `json:"mac"`
	CurrentVer   string `json:"current_ver"`
	StableVer    string `json:"stable_ver"`
	BetaVer      string `json:"beta_ver"`
	StableUpdate bool   `json:"stable_update"`
	BetaUpdate   bool   `json:"beta_update"`
	Status       string `json:"status"` // "ok", "error", "na"
	Note         string `json:"note"`
	CheckedAt    string `json:"checked_at"`
	// Identifying metadata captured opportunistically from Shelly.GetDeviceInfo
	// during the check. Empty when the GetDeviceInfo call fails or doesn't
	// return the field (older firmware). The service layer persists these
	// onto the Device row alongside the per-channel firmware cache.
	Batch string `json:"batch,omitempty"`
	FWID  string `json:"fw_id,omitempty"`
}

type UpdateResult struct {
	IP     string `json:"ip"`
	MAC    string `json:"mac"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// Options carries the per-device configuration used to build a shellyclient.
type Options struct {
	Timeout       time.Duration
	Scheme        string
	Username      string
	Password      string
	HA1           string
	AllowInsecure bool
}

func (o Options) toClientOptions() shellyclient.Options {
	out := shellyclient.Options{
		Timeout:  o.Timeout,
		Scheme:   o.Scheme,
		Username: o.Username,
		Password: o.Password,
		HA1:      o.HA1,
	}
	if o.AllowInsecure {
		out.TLSPolicy = shellyclient.TLSSkip
	}
	return out
}

// CheckOne preserves the existing signature for callers that don't yet thread
// credentials/scheme; it delegates to CheckOneWithOptions internally.
func CheckOne(ctx context.Context, d models.Device, timeout time.Duration) Result {
	return CheckOneWithOptions(ctx, d, Options{Timeout: timeout})
}

// CheckOneWithOptions issues Shelly.GetDeviceInfo to capture the running
// firmware version, then Shelly.CheckForUpdate to read the per-channel
// availability. Returning the running version here is what makes the page
// resilient to out-of-band upgrades (user flashed via the device's own web UI
// — Device.FW would otherwise stay stale). Gen1 devices are unsupported.
func CheckOneWithOptions(ctx context.Context, d models.Device, opts Options) Result {
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	res := Result{IP: d.IP, MAC: d.MAC, CurrentVer: d.FW, Status: "na", CheckedAt: checkedAt}
	if d.Gen < 2 {
		res.Note = "gen1 devices not supported"
		return res
	}
	client := shellyclient.New(opts.toClientOptions())

	// Best-effort: refresh the running version so out-of-band updates are
	// caught. A failure here is not fatal — we keep the persisted value and
	// continue with CheckForUpdate, which is the primary signal.
	if info, err := client.RPC(ctx, d.IP, "Shelly.GetDeviceInfo", nil); err == nil {
		if running := stringValue(info["ver"]); running != "" {
			res.CurrentVer = running
		} else if running := stringValue(info["fw"]); running != "" {
			res.CurrentVer = running
		}
		res.Batch = stringValue(info["batch"])
		res.FWID = stringValue(info["fw_id"])
	}

	payload, err := client.RPC(ctx, d.IP, "Shelly.CheckForUpdate", nil)
	if err != nil {
		res.Status = "error"
		res.Note = friendlyRPCError(err)
		return res
	}
	if stable, ok := payload["stable"].(map[string]any); ok {
		res.StableVer = stringValue(stable["version"])
	}
	if beta, ok := payload["beta"].(map[string]any); ok {
		res.BetaVer = stringValue(beta["version"])
	}
	res.StableUpdate = res.StableVer != "" && res.StableVer != res.CurrentVer
	res.BetaUpdate = res.BetaVer != "" && res.BetaVer != res.CurrentVer
	res.Status = "ok"
	return res
}

// TriggerUpdate retains the original signature; callers wishing to thread
// credentials/scheme should use TriggerUpdateWithOptions.
func TriggerUpdate(ctx context.Context, ip string, gen int, stage string, timeout time.Duration) UpdateResult {
	return TriggerUpdateWithOptions(ctx, ip, gen, stage, Options{Timeout: timeout})
}

func TriggerUpdateWithOptions(ctx context.Context, ip string, gen int, stage string, opts Options) UpdateResult {
	if gen < 2 {
		return UpdateResult{IP: ip, Status: "failed", Detail: "gen1 devices not supported"}
	}
	client := shellyclient.New(opts.toClientOptions())
	_, err := client.RPC(ctx, ip, "Shelly.Update", map[string]any{"stage": stage})
	if err != nil {
		if errors.Is(err, shellyclient.ErrAuthRequired) {
			return UpdateResult{IP: ip, Status: "failed", Detail: "authentication required"}
		}
		if errors.Is(err, shellyclient.ErrAuthLockout) {
			return UpdateResult{IP: ip, Status: "failed", Detail: "device locked (brute-force protection)"}
		}
		return UpdateResult{IP: ip, Status: "failed", Detail: err.Error()}
	}
	return UpdateResult{IP: ip, Status: "triggered", Detail: "update started"}
}

// GetDeviceFirmware reads the current firmware version reported by the device
// (via Shelly.GetDeviceInfo), used by the install poller to detect when a
// device has rebooted onto the new firmware. Returns "" when the call fails.
func GetDeviceFirmware(ctx context.Context, ip string, gen int, opts Options) (string, error) {
	if gen < 2 {
		return "", errors.New("gen1 devices not supported")
	}
	client := shellyclient.New(opts.toClientOptions())
	payload, err := client.RPC(ctx, ip, "Shelly.GetDeviceInfo", nil)
	if err != nil {
		return "", err
	}
	if v := stringValue(payload["ver"]); v != "" {
		return v, nil
	}
	return stringValue(payload["fw"]), nil
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// friendlyRPCError condenses the raw network/RPC error into a short phrase
// suitable for display in the Status column. The raw error often includes the
// full URL and Go-internal jargon ("context deadline exceeded ...") which is
// noisy and redundant — the IP is already shown in the row.
func friendlyRPCError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, shellyclient.ErrAuthRequired) {
		return "authentication required"
	}
	if errors.Is(err, shellyclient.ErrAuthLockout) {
		return "device locked (brute-force protection)"
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "context deadline exceeded"),
		strings.Contains(low, "client.timeout"),
		strings.Contains(low, "i/o timeout"):
		return "device did not respond in time"
	case strings.Contains(low, "connection refused"):
		return "connection refused"
	case strings.Contains(low, "no route to host"):
		return "no route to host"
	case strings.Contains(low, "no such host"):
		return "DNS lookup failed"
	}
	if len(msg) > 120 {
		return msg[:117] + "..."
	}
	return msg
}
