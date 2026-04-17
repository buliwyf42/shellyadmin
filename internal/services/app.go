package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"shellyadmin/internal/core/compliance"
	"shellyadmin/internal/core/provisioner"
	"shellyadmin/internal/core/scanner"
	"shellyadmin/internal/db"
	"shellyadmin/internal/models"
)

const (
	MaxTemplateBytes = 64 * 1024
	MaxJSONBytes     = 256 * 1024
	maxProvisionIPs  = 256
	maxSubnets       = 64
	maxScanTargets   = 65534
)

type AppService struct {
	db      *db.DB
	logf    func(level, msg string)
	dataDir string

	mu              sync.Mutex
	activeProvision map[string]bool
	activeFirmware  map[string]bool

	// ctx is cancelled by Stop; background jobs check it at progress points
	// and mark their DB row as "interrupted" before exiting.
	ctx    context.Context
	cancel context.CancelFunc
	// bgJobs tracks in-flight background goroutines (scan/refresh/firmware)
	// so Stop can drain them before returning.
	bgJobs sync.WaitGroup
}

type TemplateRecord struct {
	Name          string `json:"name"`
	Content       string `json:"content"`
	CredentialRef string `json:"credential_ref"`
}

func NewAppService(database *db.DB, dataDir string, logf func(level, msg string)) *AppService {
	ctx, cancel := context.WithCancel(context.Background())
	return &AppService{
		db:              database,
		dataDir:         dataDir,
		logf:            logf,
		activeProvision: map[string]bool{},
		activeFirmware:  map[string]bool{},
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Stop signals background jobs to exit, waits for them to drain (bounded by
// shutdownCtx), and marks any jobs still "running" as "interrupted". Safe to
// call once; subsequent calls are no-ops.
func (s *AppService) Stop(shutdownCtx context.Context) {
	s.cancel()
	done := make(chan struct{})
	go func() {
		s.bgJobs.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-shutdownCtx.Done():
		s.Log("warn", "shutdown: background jobs did not drain within timeout")
	}
	if err := s.db.MarkRunningJobsInterrupted(); err != nil {
		s.Log("error", fmt.Sprintf("shutdown: mark running jobs interrupted: %v", err))
	}
}

// linkedContext returns a context that is cancelled when either the parent
// or the service's shutdown context is cancelled.
func (s *AppService) linkedContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	go func() {
		select {
		case <-s.ctx.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func (s *AppService) GetDevices() ([]models.Device, error) {
	devices, err := s.db.ListDevices()
	if err != nil {
		return nil, err
	}
	settings, err := s.db.GetSettings()
	if err != nil {
		return nil, err
	}
	for i := range devices {
		devices[i].Compliant, devices[i].ComplianceIssues = compliance.Evaluate(devices[i], settings.Compliance)
	}
	return devices, nil
}

func (s *AppService) ForgetDevice(target string) error {
	return s.db.ForgetDevice(target)
}

func (s *AppService) RefreshDevice(ctx context.Context, target string) ([]models.Device, error) {
	devices, err := s.db.ListDevices()
	if err != nil {
		return nil, err
	}

	var current *models.Device
	for i := range devices {
		if devices[i].MAC == target || devices[i].IP == target {
			current = &devices[i]
			break
		}
	}
	if current == nil {
		return nil, fmt.Errorf("device not found")
	}

	settings, err := s.db.GetSettings()
	if err != nil {
		return nil, err
	}
	timeout := refreshProbeTimeout(settings)
	attemptedAt := time.Now().UTC().Format(time.RFC3339)
	probed := scanner.ProbeDevice(ctx, current.IP, timeout, s.Log)
	if probed == nil {
		current.LastRefreshAttempt = attemptedAt
		current.LastRefreshOK = false
		required, reason := checkAuthRequired(ctx, current.IP, timeout)
		if required {
			current.AuthRequired = true
			current.AuthError = reason
			current.LastRefreshError = reason
			current.Online = true
			current.ConsecutiveMisses = 0
		} else {
			current.LastRefreshError = "refresh timed out"
			current.ConsecutiveMisses++
			if current.ConsecutiveMisses >= 2 {
				current.Online = false
			}
		}
		if err := s.db.UpsertDevice(*current); err != nil {
			return nil, err
		}
		return s.GetDevices()
	}

	probed.DeviceNum = current.DeviceNum
	probed.FirstSeen = current.FirstSeen
	probed.LastRefreshAttempt = attemptedAt
	probed.LastRefreshOK = true
	probed.LastRefreshError = ""
	probed.ConsecutiveMisses = 0
	probed.Online = true
	probed.AuthRequired = false
	probed.AuthError = ""
	if err := s.db.UpsertDevice(*probed); err != nil {
		return nil, err
	}
	return s.GetDevices()
}

func (s *AppService) Provision(ctx context.Context, ips []string, template map[string]interface{}, credentialRef string) ([]map[string]any, error) {
	if len(ips) == 0 {
		return nil, errors.New("ips required")
	}
	if len(ips) > maxProvisionIPs {
		return nil, fmt.Errorf("too many devices requested")
	}
	if latest, err := s.db.GetLatestJob("scan"); err == nil && latest.Status == "running" {
		return nil, errors.New("provision blocked while scan is running")
	}
	for _, raw := range ips {
		addr, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid ip: %q", raw)
		}
		if !isProvisionTargetAllowed(addr) {
			return nil, fmt.Errorf("provision target %q is not in an allowed local range", raw)
		}
	}
	if err := ValidateTemplate(template); err != nil {
		return nil, err
	}
	credentialRef = strings.TrimSpace(credentialRef)
	if credentialRef != "" {
		if _, err := s.db.GetCredential(credentialRef); err != nil {
			return nil, fmt.Errorf("credential_ref %q not found", credentialRef)
		}
	}

	devices, err := s.db.ListDevices()
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
	allowedKeys, skippedKeys := s.reserveProvisionTargets(requestedKeys)
	defer s.releaseProvisionTargets(allowedKeys)

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
		info, results := provisioner.ProvisionDevice(ctx, ip, template, 10*time.Second)
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
				if uerr := s.db.UpsertDevice(device); uerr != nil {
					s.Log("error", fmt.Sprintf("provision: persist auth-required state for %s: %v", ip, uerr))
				}
			}
		}
		body, merr := json.Marshal(map[string]any{"info": info, "results": results})
		if merr != nil {
			s.Log("warn", fmt.Sprintf("provision: marshal result for %s: %v", ip, merr))
			continue
		}
		var raw map[string]any
		if uerr := json.Unmarshal(body, &raw); uerr != nil {
			s.Log("warn", fmt.Sprintf("provision: unmarshal result for %s: %v", ip, uerr))
			continue
		}
		out = append(out, raw)
	}
	return out, nil
}

func isProvisionTargetAllowed(addr netip.Addr) bool {
	// Block clearly unsafe destinations for server-side network calls.
	if addr.IsLoopback() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	// Allow only local network targets (RFC1918/ULA and link-local).
	return addr.IsPrivate() || addr.IsLinkLocalUnicast()
}

func checkAuthRequired(ctx context.Context, ip string, timeout time.Duration) (bool, string) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+ip+"/shelly", nil)
	if err != nil {
		return false, ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return true, resp.Status
	}
	return false, ""
}

func (s *AppService) GetSettings() (models.AppSettings, error) {
	return s.db.GetSettings()
}

func (s *AppService) SaveSettings(settings models.AppSettings) error {
	if err := ValidateSettings(settings); err != nil {
		return err
	}
	return s.db.SaveSettings(settings)
}

func (s *AppService) ListTemplates() ([]string, error) {
	return s.db.ListTemplateNames()
}

func (s *AppService) GetTemplate(name string) (TemplateRecord, error) {
	content, credentialRef, err := s.db.GetTemplate(name)
	if err != nil {
		return TemplateRecord{}, err
	}
	return TemplateRecord{
		Name:          name,
		Content:       content,
		CredentialRef: credentialRef,
	}, nil
}

func (s *AppService) SaveTemplate(name, content, credentialRef string) error {
	if len(content) > MaxTemplateBytes {
		return fmt.Errorf("template exceeds %d bytes", MaxTemplateBytes)
	}
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(content), &body); err != nil {
		return err
	}
	if err := ValidateTemplate(body); err != nil {
		return err
	}
	credentialRef = strings.TrimSpace(credentialRef)
	if credentialRef != "" {
		if _, err := s.db.GetCredential(credentialRef); err != nil {
			return fmt.Errorf("credential_ref %q not found", credentialRef)
		}
	}
	return s.db.SaveTemplate(name, content, credentialRef)
}

func (s *AppService) DeleteTemplate(name string) error {
	return s.db.DeleteTemplate(name)
}

func (s *AppService) GetLogs(level, search string) ([]db.LogEntry, error) {
	return s.db.GetLogs(level, search)
}

func (s *AppService) ClearLogs() (int64, error) {
	return s.db.ClearLogs()
}

func (s *AppService) Log(level, msg string) {
	s.logf(level, SanitizeLogMessage(msg))
}

func sanitizeTags(tags []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func ValidateSettings(settings models.AppSettings) error {
	settings.Normalize()
	if len(settings.Subnets) > maxSubnets {
		return fmt.Errorf("too many subnets configured")
	}
	if settings.ScanConcurrency < 1 || settings.ScanConcurrency > 256 {
		return fmt.Errorf("scan concurrency must be between 1 and 256")
	}
	if settings.ScanTimeout < 0.2 || settings.ScanTimeout > 30 {
		return fmt.Errorf("scan timeout must be between 0.2 and 30 seconds")
	}
	if settings.RefreshTimeout < 0.2 || settings.RefreshTimeout > 30 {
		return fmt.Errorf("refresh timeout must be between 0.2 and 30 seconds")
	}
	total := 0
	for _, subnet := range settings.Subnets {
		ips, err := scanner.ExpandCIDR(subnet)
		if err != nil {
			return err
		}
		total += len(ips)
	}
	if settings.EnableMDNS {
		total++
	}
	if total == 0 {
		return errors.New("no scan targets configured; add at least one subnet in Settings or enable mDNS discovery")
	}
	if total > maxScanTargets {
		return fmt.Errorf("scan target count %d exceeds limit %d", total, maxScanTargets)
	}
	if mode := settings.Compliance.WSTLSMode; mode != "" && mode != "no_validation" && mode != "default" && mode != "user" {
		return fmt.Errorf("websocket tls mode must be no_validation, default, or user")
	}
	if mode := settings.Compliance.OTAAutoUpdate; mode != "" && mode != "off" && mode != "stable" && mode != "beta" {
		return fmt.Errorf("ota auto update must be off, stable, or beta")
	}
	if settings.Compliance.RPCUDPPort != nil && *settings.Compliance.RPCUDPPort < 0 {
		return fmt.Errorf("rpc udp port must be 0 or greater")
	}
	return nil
}

func ValidateTemplate(template map[string]interface{}) error {
	body, err := json.Marshal(template)
	if err != nil {
		return err
	}
	if len(body) > MaxTemplateBytes {
		return fmt.Errorf("template exceeds %d bytes", MaxTemplateBytes)
	}
	return nil
}

var secretPattern = regexp.MustCompile(`(?i)(password|pass|secret|ha1)\s*[:=]\s*("[^"]*"|[^,\s]+)`)

func SanitizeLogMessage(msg string) string {
	return secretPattern.ReplaceAllString(msg, `$1=[redacted]`)
}

func BoundedConcurrency(value int) int {
	switch {
	case value <= 0:
		return 32
	case value > 128:
		return 128
	default:
		return value
	}
}

func boundedConcurrency(value int) int {
	return BoundedConcurrency(value)
}

func DecodeSecretValue(envKey string) string {
	if value := os.Getenv(envKey + "_FILE"); value != "" {
		body, err := os.ReadFile(value)
		if err == nil {
			return strings.TrimSpace(string(body))
		}
	}
	return strings.TrimSpace(os.Getenv(envKey))
}
