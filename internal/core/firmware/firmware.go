package firmware

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"shellyadmin/internal/core/clock"
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
	// Clock is an optional injection seam for tests. nil means real wall-clock
	// time; production callers leave this unset.
	Clock clock.Clock
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

func (o Options) clock() clock.Clock {
	if o.Clock == nil {
		return clock.Real()
	}
	return o.Clock
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
	if d.Gen < 2 {
		return Result{
			IP:         d.IP,
			MAC:        d.MAC,
			CurrentVer: d.FW,
			Status:     "na",
			CheckedAt:  opts.clock().Now().UTC().Format(time.RFC3339),
			Note:       "gen1 devices not supported",
		}
	}
	client := shellyclient.New(opts.toClientOptions())
	return CheckOneOnClient(ctx, client, d, opts.clock())
}

// CheckOneOnClient is the test seam: caller supplies a pre-built
// shellyclient (so a httptest fake-Shelly server can drive the RPC layer)
// and an explicit Clock. The gen<2 short-circuit lives in
// CheckOneWithOptions because it's a config-time check that doesn't need
// the network.
func CheckOneOnClient(ctx context.Context, client *shellyclient.Client, d models.Device, clk clock.Clock) Result {
	if clk == nil {
		clk = clock.Real()
	}
	checkedAt := clk.Now().UTC().Format(time.RFC3339)
	res := Result{IP: d.IP, MAC: d.MAC, CurrentVer: d.FW, Status: "na", CheckedAt: checkedAt}

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
	res.StableUpdate = res.StableVer != "" && IsNewer(res.StableVer, res.CurrentVer)
	res.BetaUpdate = res.BetaVer != "" && IsNewer(res.BetaVer, res.CurrentVer)
	res.Status = "ok"
	return res
}

// IsNewer reports whether candidate is a strictly newer version than current.
//
// This must not be a string comparison. A device running a beta sits *ahead* of
// its model's stable channel (2.0.0-beta3 vs stable 1.7.5 during the phased
// 2.0.0 rollout), and "differs from current" would advertise that older stable
// as an available update. Shelly.Update then accepts the call and silently does
// nothing, which surfaces as a five-minute wait ending in "unknown".
//
// Shelly versions are semver-shaped ("2.0.0", "2.0.0-beta3", "1.7.99-plugmg3prod0"),
// so semver decides the ordering — including prerelease < release, which is the
// case that matters here. Unparseable versions fall back to the old
// string-inequality behaviour so an odd vendor string can never *hide* a real
// update; it can only fail to suppress a downgrade.
func IsNewer(candidate, current string) bool {
	c, cur := "v"+candidate, "v"+current
	if !semver.IsValid(c) || !semver.IsValid(cur) {
		return candidate != current
	}
	return semver.Compare(c, cur) > 0
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
	return TriggerUpdateOnClient(ctx, client, ip, stage)
}

// TriggerUpdateOnClient is the test seam: caller supplies a pre-built
// shellyclient. No clock dependency — the gen<2 short-circuit happens in
// TriggerUpdateWithOptions before this is reached.
func TriggerUpdateOnClient(ctx context.Context, client *shellyclient.Client, ip, stage string) UpdateResult {
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
	return GetDeviceFirmwareOnClient(ctx, client, ip)
}

// GetDeviceFirmwareOnClient is the test seam. No clock dependency.
func GetDeviceFirmwareOnClient(ctx context.Context, client *shellyclient.Client, ip string) (string, error) {
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
