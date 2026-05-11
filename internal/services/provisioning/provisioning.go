// Package provisioning hosts the operator-driven template provisioning
// flow (multi-device Shelly RPC configuration) and the user-CA / TLS
// client cert upload path. Both share the provision/firmware target
// reservation slot (via Host callbacks) so a Provision can't collide
// with a concurrent firmware update on the same target.
//
// MOVED FROM internal/services/app.go — v0.3.0 services-layer split (M7,
// docs/plans/phase-4b-refactor-block.md Block 4b.1). AppService keeps
// delegators on Provision / UploadUserCA so API handlers and tests are
// unchanged.
package provisioning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"shellyadmin/internal/core/provisioner"
	"shellyadmin/internal/models"
	"shellyadmin/internal/services/validation"
)

// MaxProvisionIPs caps the per-call target list. Same value as the
// services-level maxProvisionIPs.
const MaxProvisionIPs = 256

// MaxUserCABytes caps the PEM payload size accepted by UploadUserCA. A
// single CA bundle is rarely larger than a few KB; 64KB is comfortably
// above realistic certificate chains while bounding server memory use.
const MaxUserCABytes = 64 * 1024

// UploadUserCAResult reports a single-device user-CA upload outcome for
// the HTTP API (one entry per requested IP).
type UploadUserCAResult struct {
	IP        string `json:"ip"`
	Status    string `json:"status"`
	Chunks    int    `json:"chunks"`
	BytesSent int    `json:"bytes_sent"`
	Detail    string `json:"detail"`
}

// cloudMetadataAddr is the AWS/GCP/Azure/DO cloud metadata endpoint at
// 169.254.169.254 — RFC3927 link-local space, so it would slip past
// addr.IsLinkLocalUnicast() even though leaking a request to it from
// ShellyAdmin would be a credential-disclosure SSRF (M5 in the
// consolidated review). The container never has a legitimate reason to
// reach it; explicitly deny.
var cloudMetadataAddr = netip.MustParseAddr("169.254.169.254")

// IsTargetAllowed reports whether addr is in the operator-controllable
// local network space the provision/upload paths may reach.
func IsTargetAllowed(addr netip.Addr) bool {
	if addr.IsLoopback() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	if addr == cloudMetadataAddr {
		return false
	}
	return addr.IsPrivate() || addr.IsLinkLocalUnicast()
}

// Store is the narrow persistence surface provisioning needs.
type Store interface {
	GetLatestJob(jobType string) (models.Job, error)
	ListDevices() ([]models.Device, error)
	UpsertDevice(d models.Device) error
	GetCredential(name string) (models.Credential, error)
}

// Host is the runtime surface (logging + concurrency reservation + RPC
// option builder). *AppService implements it.
type Host interface {
	LogCtx(ctx context.Context, level, msg string)
	ReserveProvisionTargets(requested []string) (allowed []string, skipped []string)
	ReleaseProvisionTargets(keys []string)
	ProvisionOptions(d models.Device, credentialRef string, timeout time.Duration) provisioner.Options
}

// Service hosts Provision + UploadUserCA.
type Service struct {
	store Store
	host  Host
}

// New constructs a Service backed by the given Store + Host.
func New(store Store, host Host) *Service {
	return &Service{store: store, host: host}
}

// Provision applies a template (the v2 JSON schema understood by
// internal/core/provisioner) to each IP. Returns per-device result rows
// in the same order as ips, with precheck-skipped + busy-skipped
// entries interleaved so callers can render a complete report.
func (s *Service) Provision(ctx context.Context, ips []string, template map[string]interface{}, credentialRef string) ([]map[string]any, error) {
	if len(ips) == 0 {
		return nil, errors.New("ips required")
	}
	if len(ips) > MaxProvisionIPs {
		return nil, fmt.Errorf("too many devices requested")
	}
	if latest, err := s.store.GetLatestJob("scan"); err == nil && latest.Status == "running" {
		return nil, errors.New("provision blocked while scan is running")
	}
	for _, raw := range ips {
		addr, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid ip: %q", raw)
		}
		if !IsTargetAllowed(addr) {
			return nil, fmt.Errorf("provision target %q is not in an allowed local range", raw)
		}
	}
	if err := validation.Template(template); err != nil {
		return nil, err
	}
	credentialRef = strings.TrimSpace(credentialRef)
	if credentialRef != "" {
		if _, err := s.store.GetCredential(credentialRef); err != nil {
			return nil, fmt.Errorf("credential_ref %q not found", credentialRef)
		}
	}

	devices, err := s.store.ListDevices()
	if err != nil {
		return nil, err
	}
	ipToDevice := map[string]models.Device{}
	ipToKey := map[string]string{}
	for _, device := range devices {
		ipToDevice[device.IP] = device
		key := "ip:" + device.IP
		if device.MAC != "" {
			key = "mac:" + device.MAC
		}
		ipToKey[device.IP] = key
	}
	requestedKeys := make([]string, 0, len(ips))
	keyToIP := map[string]string{}
	precheckSkipped := []map[string]any{}
	for _, ip := range ips {
		device, known := ipToDevice[ip]
		if known && device.AuthRequired && credentialRef == "" {
			precheckSkipped = append(precheckSkipped, map[string]any{
				"info": map[string]any{"ip": ip},
				"results": []map[string]any{
					{"section": "precheck", "status": "skipped", "detail": "auth required but credential_ref is missing"},
				},
			})
			continue
		}
		key := ipToKey[ip]
		if key == "" {
			key = "ip:" + ip
		}
		requestedKeys = append(requestedKeys, key)
		keyToIP[key] = ip
	}
	allowedKeys, skippedKeys := s.host.ReserveProvisionTargets(requestedKeys)
	defer s.host.ReleaseProvisionTargets(allowedKeys)

	allowed := make([]string, 0, len(allowedKeys))
	for _, key := range allowedKeys {
		allowed = append(allowed, keyToIP[key])
	}

	out := make([]map[string]any, 0, len(ips))
	out = append(out, precheckSkipped...)
	for _, skipped := range skippedKeys {
		out = append(out, map[string]any{
			"info": map[string]any{
				"ip": keyToIP[skipped],
			},
			"results": []map[string]any{
				{"section": "precheck", "status": "skipped", "detail": "device busy with firmware update"},
			},
		})
	}
	for _, ip := range allowed {
		device := ipToDevice[ip]
		device.IP = ip // ensure populated for fresh devices
		opts := s.host.ProvisionOptions(device, credentialRef, 10*time.Second)
		info, results := provisioner.ProvisionDeviceWithOptions(ctx, ip, template, opts)
		authRequired := false
		authReason := ""
		for _, section := range results {
			if section.Status == "failed" && (strings.Contains(section.Detail, "401") || strings.Contains(section.Detail, "403")) {
				authRequired = true
				authReason = section.Detail
				break
			}
		}
		if authRequired {
			if device, ok := ipToDevice[ip]; ok {
				device.AuthRequired = true
				device.AuthError = authReason
				if uerr := s.store.UpsertDevice(device); uerr != nil {
					s.host.LogCtx(ctx, "error", fmt.Sprintf("provision: persist auth-required state for %s: %v", ip, uerr))
				}
			}
		}
		restartRequired := false
		for _, r := range results {
			if r.RestartRequired {
				restartRequired = true
				break
			}
		}
		body, merr := json.Marshal(map[string]any{"info": info, "results": results, "restart_required": restartRequired})
		if merr != nil {
			s.host.LogCtx(ctx, "warn", fmt.Sprintf("provision: marshal result for %s: %v", ip, merr))
			continue
		}
		var raw map[string]any
		if uerr := json.Unmarshal(body, &raw); uerr != nil {
			s.host.LogCtx(ctx, "warn", fmt.Sprintf("provision: unmarshal result for %s: %v", ip, uerr))
			continue
		}
		out = append(out, raw)
	}
	return out, nil
}

// UploadUserCA sends a PEM-encoded certificate (user CA, TLS client cert,
// or TLS client key, selected by kind) to one or more devices via chunked
// Shelly.Put* RPCs. Targets are validated the same way Provision validates
// IPs (local network only) and reserved through the Provision/FirmwareUpdate
// exclusion slot so concurrent jobs can't collide on the same device.
//
// An empty kind defaults to "user_ca" for back-compat with original
// callers.
func (s *Service) UploadUserCA(ctx context.Context, ips []string, kind string, pem string) ([]UploadUserCAResult, error) {
	if len(ips) == 0 {
		return nil, errors.New("ips required")
	}
	if len(ips) > MaxProvisionIPs {
		return nil, fmt.Errorf("too many devices requested")
	}
	certKind, err := provisioner.ParseCertificateKind(kind)
	if err != nil {
		return nil, err
	}
	pem = strings.TrimSpace(pem)
	if pem == "" {
		return nil, errors.New("pem is required")
	}
	if len(pem) > MaxUserCABytes {
		return nil, fmt.Errorf("pem exceeds %d byte limit", MaxUserCABytes)
	}
	if !strings.Contains(pem, "-----BEGIN") {
		return nil, errors.New("pem must contain a PEM header (-----BEGIN ...-----)")
	}
	if latest, err := s.store.GetLatestJob("scan"); err == nil && latest.Status == "running" {
		return nil, errors.New("certificate upload blocked while scan is running")
	}
	normalized := make([]string, 0, len(ips))
	for _, raw := range ips {
		addr, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid ip: %q", raw)
		}
		if !IsTargetAllowed(addr) {
			return nil, fmt.Errorf("user-ca target %q is not in an allowed local range", raw)
		}
		normalized = append(normalized, strings.TrimSpace(raw))
	}

	devices, err := s.store.ListDevices()
	if err != nil {
		return nil, err
	}
	ipToKey := map[string]string{}
	for _, device := range devices {
		key := "ip:" + device.IP
		if device.MAC != "" {
			key = "mac:" + device.MAC
		}
		ipToKey[device.IP] = key
	}
	requestedKeys := make([]string, 0, len(normalized))
	keyToIP := map[string]string{}
	for _, ip := range normalized {
		key, ok := ipToKey[ip]
		if !ok {
			key = "ip:" + ip
		}
		requestedKeys = append(requestedKeys, key)
		keyToIP[key] = ip
	}
	allowedKeys, skippedKeys := s.host.ReserveProvisionTargets(requestedKeys)
	defer s.host.ReleaseProvisionTargets(allowedKeys)

	results := make([]UploadUserCAResult, 0, len(normalized))
	for _, key := range skippedKeys {
		results = append(results, UploadUserCAResult{
			IP:     keyToIP[key],
			Status: "skipped",
			Detail: "device busy with firmware update",
		})
	}
	for _, key := range allowedKeys {
		ip := keyToIP[key]
		res, err := provisioner.UploadCertificate(ctx, ip, certKind, pem, 20*time.Second)
		entry := UploadUserCAResult{
			IP:        ip,
			Chunks:    res.Chunks,
			BytesSent: res.BytesSent,
		}
		if err != nil {
			entry.Status = "failed"
			entry.Detail = err.Error()
			s.host.LogCtx(ctx, "warn", fmt.Sprintf("%s upload to %s failed: %v", certKind, ip, err))
		} else {
			entry.Status = "ok"
			entry.Detail = res.Detail
			s.host.LogCtx(ctx, "info", fmt.Sprintf("%s uploaded to %s: %d chunks, %d bytes", certKind, ip, res.Chunks, res.BytesSent))
		}
		results = append(results, entry)
	}
	return results, nil
}
