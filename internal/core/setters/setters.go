// Package setters issues targeted single-field RPCs (location, MQTT server,
// reboot, etc.) used by the bulk-action surface. Each call routes through a
// shellyclient.Client so digest auth, TLS, and brute-force lockout signalling
// are handled uniformly with the rest of the device-talking code paths.
package setters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"shellyadmin/internal/core/shellyclient"
)

// Options carries the per-device configuration used to build a shellyclient.
// Empty values are sensible defaults (http, no auth, 5s timeout).
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

// Setter encapsulates a single shellyclient.Client so callers can issue
// multiple targeted setter calls (e.g. when applying several bulk actions in
// sequence) without re-establishing auth state on each call.
type Setter struct {
	c *shellyclient.Client
}

// New builds a Setter from the given Options. Reuse across calls to the same device.
func New(opts Options) *Setter { return &Setter{c: shellyclient.New(opts.toClientOptions())} }

// NewWithClient is the test seam: it wraps a pre-built shellyclient (so a
// httptest fake-Shelly server can drive the RPC layer) without going
// through Options. Production callers should use New.
func NewWithClient(c *shellyclient.Client) *Setter { return &Setter{c: c} }

func (s *Setter) call(ctx context.Context, ip, method string, payload map[string]any) bool {
	params := payload
	if isSetConfig(method) {
		params = map[string]any{"config": payload}
	}
	_, err := s.c.RPC(ctx, ip, method, params)
	if err == nil {
		return true
	}
	// 404/-32601 means the method is not available on this model. We don't
	// silently treat that as success — bulk actions should report it back to
	// the caller so the UI can show "skipped, not supported on this model".
	if shellyclient.IsMethodNotFound(err) {
		return false
	}
	if errors.Is(err, shellyclient.ErrAuthRequired) || errors.Is(err, shellyclient.ErrAuthLockout) {
		return false
	}
	return false
}

func isSetConfig(method string) bool {
	return len(method) > 9 && method[len(method)-9:] == "SetConfig"
}

func (s *Setter) SetLocation(ctx context.Context, ip string, lat, lon float64) bool {
	return s.call(ctx, ip, "Sys.SetConfig", map[string]any{"location": map[string]any{"lat": lat, "lon": lon}})
}

func (s *Setter) SetTimezone(ctx context.Context, ip, tz string) bool {
	return s.call(ctx, ip, "Sys.SetConfig", map[string]any{"location": map[string]any{"tz": tz}})
}

func (s *Setter) SetMQTTServer(ctx context.Context, ip, server string) bool {
	return s.call(ctx, ip, "MQTT.SetConfig", map[string]any{"server": server})
}

func (s *Setter) SetMQTTEnabled(ctx context.Context, ip string, enabled bool) bool {
	return s.call(ctx, ip, "MQTT.SetConfig", map[string]any{"enable": enabled})
}

func (s *Setter) SetSNTPServer(ctx context.Context, ip, server string) bool {
	return s.call(ctx, ip, "Sys.SetConfig", map[string]any{"sntp": map[string]any{"server": server}})
}

func (s *Setter) SetCloudEnabled(ctx context.Context, ip string, enabled bool) bool {
	return s.call(ctx, ip, "Cloud.SetConfig", map[string]any{"enable": enabled})
}

// SetBLEEnabled retains the legacy semantics. On Shelly firmware ≥ 2.0.0-beta1
// the global enable flag was removed and devices may return a 404 RPC error;
// the call returns false in that case so callers report "not supported".
func (s *Setter) SetBLEEnabled(ctx context.Context, ip string, enabled bool) bool {
	return s.call(ctx, ip, "BLE.SetConfig", map[string]any{"enable": enabled})
}

// SetWiFiHostname configures the per-device hostname (FW 2.0.0-beta1).
// Older firmwares may return method-not-found, in which case this returns false.
func (s *Setter) SetWiFiHostname(ctx context.Context, ip, hostname string) bool {
	return s.call(ctx, ip, "Wifi.SetConfig", map[string]any{"sta": map[string]any{"hostname": hostname}})
}

// SetCoverTilt sets a slatted-cover tilt position (0–100 percent), used by the
// venetian-blinds support added in FW 2.0.0-beta1. Devices without slat
// support return method-not-found and the call returns false.
func (s *Setter) SetCoverTilt(ctx context.Context, ip string, id int, percent int) bool {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	_, err := s.c.RPC(ctx, ip, "Cover.GoToTilt", map[string]any{"id": id, "pos": percent})
	return err == nil
}

func (s *Setter) Reboot(ctx context.Context, ip string) bool {
	_, err := s.c.RPC(ctx, ip, "Shelly.Reboot", nil)
	return err == nil
}

// CoverOpen / CoverClose / CoverStop dispatch the corresponding RPC for the
// given Cover component instance. Most Shelly cover devices have a single
// instance (id=0); the per-component action discovery layer (ADR-0010) is
// what produces multiple action rows when the device exposes multiple.
func (s *Setter) CoverOpen(ctx context.Context, ip string, id int) (bool, string) {
	if _, err := s.c.RPC(ctx, ip, "Cover.Open", map[string]any{"id": id}); err != nil {
		return false, err.Error()
	}
	return true, fmt.Sprintf("cover %d opening", id)
}

func (s *Setter) CoverClose(ctx context.Context, ip string, id int) (bool, string) {
	if _, err := s.c.RPC(ctx, ip, "Cover.Close", map[string]any{"id": id}); err != nil {
		return false, err.Error()
	}
	return true, fmt.Sprintf("cover %d closing", id)
}

func (s *Setter) CoverStop(ctx context.Context, ip string, id int) (bool, string) {
	if _, err := s.c.RPC(ctx, ip, "Cover.Stop", map[string]any{"id": id}); err != nil {
		return false, err.Error()
	}
	return true, fmt.Sprintf("cover %d stopped", id)
}

// SwitchToggle / LightToggle flip the on/off state of a single component
// instance. The `Switch.Toggle` and `Light.Toggle` RPCs are read-modify-
// write on the device side — no need to fetch current state first.
func (s *Setter) SwitchToggle(ctx context.Context, ip string, id int) (bool, string) {
	if _, err := s.c.RPC(ctx, ip, "Switch.Toggle", map[string]any{"id": id}); err != nil {
		return false, err.Error()
	}
	return true, fmt.Sprintf("switch %d toggled", id)
}

func (s *Setter) LightToggle(ctx context.Context, ip string, id int) (bool, string) {
	if _, err := s.c.RPC(ctx, ip, "Light.Toggle", map[string]any{"id": id}); err != nil {
		return false, err.Error()
	}
	return true, fmt.Sprintf("light %d toggled", id)
}

// OTARevert rolls the device back to the previously-installed firmware.
// High-risk: the firmware may have been replaced for a reason. Surfaced
// behind the typed-name confirm modal in the action-discovery UI.
func (s *Setter) OTARevert(ctx context.Context, ip string) (bool, string) {
	if _, err := s.c.RPC(ctx, ip, "OTA.Revert", nil); err != nil {
		return false, err.Error()
	}
	return true, "firmware rollback triggered"
}

// FactoryReset wipes every persisted configuration value on the device.
// Unrecoverable from the app side — operator must re-provision afterward.
// Returns (ok, error-message); the per-device action layer converts these
// to DeviceActionResult statuses.
func (s *Setter) FactoryReset(ctx context.Context, ip string) (bool, string) {
	if _, err := s.c.RPC(ctx, ip, "Shelly.FactoryReset", nil); err != nil {
		return false, err.Error()
	}
	return true, "factory reset triggered"
}

// ResetWiFiConfig clears stored Wi-Fi + cloud credentials but keeps the
// rest of the device config (scripts, KVS, schedule, etc.). The device
// returns to the AP-mode "ready to be re-provisioned" state.
func (s *Setter) ResetWiFiConfig(ctx context.Context, ip string) (bool, string) {
	if _, err := s.c.RPC(ctx, ip, "Shelly.ResetWiFiConfig", nil); err != nil {
		return false, err.Error()
	}
	return true, "Wi-Fi & cloud config reset"
}

// WiFiScan asks the device to scan its radio for visible SSIDs. Result
// payload is forwarded to the operator UI as-is so signal strength /
// channel / encryption type can be surfaced for diagnostics.
func (s *Setter) WiFiScan(ctx context.Context, ip string) (map[string]any, error) {
	return s.c.RPC(ctx, ip, "Wifi.Scan", nil)
}

// EthGetStatus reads the Ethernet component status: link state, IPv4 + IPv6
// addresses, etc. Cheap diagnostic for "is the wire actually up".
func (s *Setter) EthGetStatus(ctx context.Context, ip string) (map[string]any, error) {
	return s.c.RPC(ctx, ip, "Eth.GetStatus", nil)
}

// BLEPair triggers BLE pairing mode (FW 2.0.0-beta1). Devices on older firmware
// return method-not-found and the action surfaces "not supported on this firmware".
// Returns (ok, supported, error-message). supported=false means the device's
// firmware doesn't expose the RPC (404 / -32601); the UI should treat that as
// a soft no-op rather than a failure.
func (s *Setter) BLEPair(ctx context.Context, ip string) (ok bool, supported bool, message string) {
	_, err := s.c.RPC(ctx, ip, "BLE.Pair", nil)
	if err == nil {
		return true, true, "pairing started"
	}
	if shellyclient.IsMethodNotFound(err) {
		return false, false, "BLE.Pair not supported on this firmware"
	}
	if errors.Is(err, shellyclient.ErrAuthRequired) {
		return false, true, "authentication required"
	}
	if errors.Is(err, shellyclient.ErrAuthLockout) {
		return false, true, "device locked (brute-force protection)"
	}
	return false, true, err.Error()
}

// ----- backward-compatible package-level wrappers -----

func SetLocation(ctx context.Context, ip string, lat, lon float64, timeout time.Duration) bool {
	return New(Options{Timeout: timeout}).SetLocation(ctx, ip, lat, lon)
}
func SetTimezone(ctx context.Context, ip, tz string, timeout time.Duration) bool {
	return New(Options{Timeout: timeout}).SetTimezone(ctx, ip, tz)
}
func SetMQTTServer(ctx context.Context, ip, server string, timeout time.Duration) bool {
	return New(Options{Timeout: timeout}).SetMQTTServer(ctx, ip, server)
}
func SetMQTTEnabled(ctx context.Context, ip string, enabled bool, timeout time.Duration) bool {
	return New(Options{Timeout: timeout}).SetMQTTEnabled(ctx, ip, enabled)
}
func SetSNTPServer(ctx context.Context, ip, server string, timeout time.Duration) bool {
	return New(Options{Timeout: timeout}).SetSNTPServer(ctx, ip, server)
}
func SetCloudEnabled(ctx context.Context, ip string, enabled bool, timeout time.Duration) bool {
	return New(Options{Timeout: timeout}).SetCloudEnabled(ctx, ip, enabled)
}
func SetBLEEnabled(ctx context.Context, ip string, enabled bool, timeout time.Duration) bool {
	return New(Options{Timeout: timeout}).SetBLEEnabled(ctx, ip, enabled)
}
func Reboot(ctx context.Context, ip string, timeout time.Duration) bool {
	return New(Options{Timeout: timeout}).Reboot(ctx, ip)
}
