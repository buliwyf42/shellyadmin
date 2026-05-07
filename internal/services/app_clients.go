package services

import (
	"context"
	"strings"
	"time"

	"shellyadmin/internal/core/firmware"
	"shellyadmin/internal/core/provisioner"
	"shellyadmin/internal/core/scanner"
	"shellyadmin/internal/core/setters"
	"shellyadmin/internal/models"
)

// resolveDeviceCredential looks up the credential (if any) assigned to a device
// via the credential-group → credential pipeline. Returns an empty struct and
// false when no assignment exists; callers fall back to unauthenticated calls.
//
// Lookup order:
//  1. device_credential_groups.group_name → credential_groups.credential_ref
//  2. fallback: a credential whose name matches the group_name directly (the
//     SaveCredentialGroup helper writes a mirror credential of the same name).
func (s *AppService) resolveDeviceCredential(mac string) (models.Credential, bool) {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return models.Credential{}, false
	}
	assignments, err := s.db.ListDeviceCredentialGroupAssignments()
	if err != nil {
		return models.Credential{}, false
	}
	var groupName string
	for _, a := range assignments {
		if a.MAC == mac {
			groupName = a.GroupName
			break
		}
	}
	if groupName == "" {
		return models.Credential{}, false
	}
	if cred, err := s.db.GetCredential(groupName); err == nil {
		return cred, true
	}
	return models.Credential{}, false
}

// schemeForDevice picks the scheme to talk to the device: "https" if the
// previous probe upgraded (recorded on the device row), else "http".
func schemeForDevice(d models.Device) string {
	scheme := strings.TrimSpace(d.Scheme)
	if scheme == "" {
		return "http"
	}
	return scheme
}

// allowInsecureForDevice returns true when the operator has explicitly opted
// out of TLS verification for this device.
func allowInsecureForDevice(d models.Device) bool {
	return d.TLSAllowInsecure != nil && *d.TLSAllowInsecure
}

// scannerProbeOptions composes a scanner.ProbeOptions for the given device,
// pulling credentials and TLS policy from persistent state. KnownMAC is
// populated so recoverable probe failures (auth-required, lockout, TLS-cert
// invalid) get persisted on the existing device row rather than dropped.
func (s *AppService) scannerProbeOptions(d models.Device, timeout time.Duration) scanner.ProbeOptions {
	opts := scanner.ProbeOptions{
		Timeout:       timeout,
		Scheme:        schemeForDevice(d),
		AllowInsecure: allowInsecureForDevice(d),
		KnownMAC:      d.MAC,
	}
	if cred, ok := s.resolveDeviceCredential(d.MAC); ok {
		opts.Username = cred.Username
		opts.Password = cred.Password
		opts.HA1 = cred.HA1
	}
	return opts
}

func (s *AppService) setterOptions(d models.Device, timeout time.Duration) setters.Options {
	opts := setters.Options{
		Timeout:       timeout,
		Scheme:        schemeForDevice(d),
		AllowInsecure: allowInsecureForDevice(d),
	}
	if cred, ok := s.resolveDeviceCredential(d.MAC); ok {
		opts.Username = cred.Username
		opts.Password = cred.Password
		opts.HA1 = cred.HA1
	}
	return opts
}

// refreshDeviceCapabilities pulls per-channel firmware availability, the
// auto-update schedule, and the Shelly.ListMethods cache for one device, in
// place, so a Refresh keeps everything that runFirmwareJob would have
// written. Best-effort — failures leave the existing persisted fields
// intact (so a transient cloud blip doesn't blank the cache).
func (s *AppService) refreshDeviceCapabilities(ctx context.Context, d *models.Device) {
	if d == nil || d.Gen < 2 || !d.Online || d.AuthRequired {
		return
	}
	fwOpts := s.firmwareOptions(*d, 5*time.Second)
	if r := firmware.CheckOneWithOptions(ctx, *d, fwOpts); r.Status == "ok" {
		if r.CurrentVer != "" {
			d.FW = r.CurrentVer
		}
		d.FWAvailableStable = r.StableVer
		d.FWAvailableBeta = r.BetaVer
		d.FWCheckedAt = r.CheckedAt
	}
	if mode, err := firmware.ReadAutoUpdate(ctx, d.IP, d.Gen, fwOpts); err == nil {
		d.FWAutoUpdate = mode
	}
	if methods, err := firmware.ListSupportedMethods(ctx, d.IP, d.Gen, fwOpts); err == nil {
		d.SupportedMethods = methods
	}
}

func (s *AppService) firmwareOptions(d models.Device, timeout time.Duration) firmware.Options {
	opts := firmware.Options{
		Timeout:       timeout,
		Scheme:        schemeForDevice(d),
		AllowInsecure: allowInsecureForDevice(d),
	}
	if cred, ok := s.resolveDeviceCredential(d.MAC); ok {
		opts.Username = cred.Username
		opts.Password = cred.Password
		opts.HA1 = cred.HA1
	}
	return opts
}

// provisionOptions builds a provisioner.Options for the given device. The
// credentialRef from the caller (template-level credential) takes precedence
// over the per-device credential mapping; callers without a ref fall back to
// the device's assigned credential.
func (s *AppService) provisionOptions(d models.Device, credentialRef string, timeout time.Duration) provisioner.Options {
	opts := provisioner.Options{
		Timeout:       timeout,
		Scheme:        schemeForDevice(d),
		AllowInsecure: allowInsecureForDevice(d),
	}
	credentialRef = strings.TrimSpace(credentialRef)
	if credentialRef != "" {
		if cred, err := s.db.GetCredential(credentialRef); err == nil {
			opts.Username = cred.Username
			opts.Password = cred.Password
			opts.HA1 = cred.HA1
			return opts
		}
	}
	if cred, ok := s.resolveDeviceCredential(d.MAC); ok {
		opts.Username = cred.Username
		opts.Password = cred.Password
		opts.HA1 = cred.HA1
	}
	return opts
}
