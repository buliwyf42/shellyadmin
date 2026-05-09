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
	"shellyadmin/internal/core/secretbox"
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
	db      Store
	logf    func(ctx context.Context, level, msg string)
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
	// stopOnce guards Stop so repeated invocations (e.g. from overlapping
	// signal handlers) don't double-cancel or re-mark interrupted jobs.
	stopOnce sync.Once
}

type TemplateRecord struct {
	Name          string `json:"name"`
	Content       string `json:"content"`
	CredentialRef string `json:"credential_ref"`
}

func NewAppService(database Store, dataDir string, logf func(ctx context.Context, level, msg string)) *AppService {
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

// StartBackgroundWorkers spawns the long-lived background goroutines owned
// by this service (currently just the firmware-check scheduler). Called once
// at startup from main.go after RecoverInterruptedJobs. Goroutines exit on
// service Stop and are awaited via s.bgJobs.
func (s *AppService) StartBackgroundWorkers() {
	s.bgJobs.Add(1)
	go func() {
		defer s.bgJobs.Done()
		s.runFirmwareCheckScheduler()
	}()
}

// Stop signals background jobs to exit, waits for them to drain (bounded by
// shutdownCtx), and marks any jobs still "running" as "interrupted". Safe to
// call once; subsequent calls are no-ops.
func (s *AppService) Stop(shutdownCtx context.Context) {
	s.stopOnce.Do(func() {
		s.cancel()
		done := make(chan struct{})
		go func() {
			s.bgJobs.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-shutdownCtx.Done():
			s.LogCtx(shutdownCtx, "warn", "shutdown: background jobs did not drain within timeout")
		}
		if err := s.db.MarkRunningJobsInterrupted(); err != nil {
			s.LogCtx(shutdownCtx, "error", fmt.Sprintf("shutdown: mark running jobs interrupted: %v", err))
		}
	})
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
		devices[i].SwitchCount = len(componentInstances(devices[i], "switch"))
		devices[i].CoverCount = len(componentInstances(devices[i], "cover"))
		devices[i].LightCount = len(componentInstances(devices[i], "light"))
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
	opts := s.scannerProbeOptions(*current, timeout)
	probed := scanner.ProbeDeviceWithOptions(ctx, current.IP, opts, s.Log)
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

	// Probe may return a partial device (auth-required / locked / TLS-bad)
	// when the underlying error is recoverable but the full snapshot is not
	// available. In that case, persist the failure state and keep the
	// existing rich fields from `current`.
	if probed.AuthRequired || probed.AuthLockedUntil != "" || (probed.TLSCertValid != nil && !*probed.TLSCertValid) {
		current.LastRefreshAttempt = attemptedAt
		current.LastRefreshOK = false
		current.AuthRequired = probed.AuthRequired
		current.AuthError = probed.AuthError
		if probed.AuthLockedUntil != "" {
			current.AuthLockedUntil = probed.AuthLockedUntil
		}
		if probed.TLSCertValid != nil {
			current.TLSCertValid = probed.TLSCertValid
		}
		current.LastRefreshError = probed.AuthError
		current.Online = true
		current.ConsecutiveMisses = 0
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
	probed.AuthLockedUntil = ""
	// Carry forward operator-set TLS opt-out — it isn't reported by the device.
	probed.TLSAllowInsecure = current.TLSAllowInsecure
	// Carry forward the firmware cache so a Refresh that fails to re-check
	// firmware (e.g. transient cloud blip) doesn't blank out the fields. The
	// helper below overwrites these on success.
	probed.FWAvailableStable = current.FWAvailableStable
	probed.FWAvailableBeta = current.FWAvailableBeta
	probed.FWCheckedAt = current.FWCheckedAt
	probed.FWAutoUpdate = current.FWAutoUpdate
	s.refreshDeviceCapabilities(ctx, probed)
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
		device := ipToDevice[ip]
		device.IP = ip // ensure populated for fresh devices
		opts := s.provisionOptions(device, credentialRef, 10*time.Second)
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
				if uerr := s.db.UpsertDevice(device); uerr != nil {
					s.LogCtx(ctx, "error", fmt.Sprintf("provision: persist auth-required state for %s: %v", ip, uerr))
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
			s.LogCtx(ctx, "warn", fmt.Sprintf("provision: marshal result for %s: %v", ip, merr))
			continue
		}
		var raw map[string]any
		if uerr := json.Unmarshal(body, &raw); uerr != nil {
			s.LogCtx(ctx, "warn", fmt.Sprintf("provision: unmarshal result for %s: %v", ip, uerr))
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

// MaxUserCABytes caps the PEM payload size accepted by UploadUserCA. A
// single CA bundle is rarely larger than a few KB; 64KB is comfortably above
// realistic certificate chains while bounding server memory use.
const MaxUserCABytes = 64 * 1024

// UploadUserCAResult reports a single-device user CA upload outcome for the
// HTTP API (one entry per requested IP).
type UploadUserCAResult struct {
	IP        string `json:"ip"`
	Status    string `json:"status"`
	Chunks    int    `json:"chunks"`
	BytesSent int    `json:"bytes_sent"`
	Detail    string `json:"detail"`
}

// UploadUserCA sends a PEM-encoded certificate (user CA, TLS client cert, or
// TLS client key, selected by kind) to one or more devices via chunked
// Shelly.Put* RPCs. Targets are validated the same way Provision validates
// IPs (local network only) and reserved through the Provision/FirmwareUpdate
// exclusion slot so concurrent jobs can't collide on the same device.
//
// An empty kind defaults to "user_ca" for back-compat with original callers.
func (s *AppService) UploadUserCA(ctx context.Context, ips []string, kind string, pem string) ([]UploadUserCAResult, error) {
	if len(ips) == 0 {
		return nil, errors.New("ips required")
	}
	if len(ips) > maxProvisionIPs {
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
	if latest, err := s.db.GetLatestJob("scan"); err == nil && latest.Status == "running" {
		return nil, errors.New("certificate upload blocked while scan is running")
	}
	normalized := make([]string, 0, len(ips))
	for _, raw := range ips {
		addr, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid ip: %q", raw)
		}
		if !isProvisionTargetAllowed(addr) {
			return nil, fmt.Errorf("user-ca target %q is not in an allowed local range", raw)
		}
		normalized = append(normalized, strings.TrimSpace(raw))
	}

	// Resolve each IP to its MAC (if known) so the reservation key matches the
	// one Provision/FirmwareUpdate use; fall back to "ip:<addr>" for unknown
	// devices. Mirrors the pattern in Provision (app.go:216-246).
	devices, err := s.db.ListDevices()
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
	allowedKeys, skippedKeys := s.reserveProvisionTargets(requestedKeys)
	defer s.releaseProvisionTargets(allowedKeys)

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
			s.LogCtx(ctx, "warn", fmt.Sprintf("%s upload to %s failed: %v", certKind, ip, err))
		} else {
			entry.Status = "ok"
			entry.Detail = res.Detail
			s.LogCtx(ctx, "info", fmt.Sprintf("%s uploaded to %s: %d chunks, %d bytes", certKind, ip, res.Chunks, res.BytesSent))
		}
		results = append(results, entry)
	}
	return results, nil
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

// MCPTokenRedacted is the placeholder API GET handlers substitute for a
// non-empty MCP token before returning settings to the SPA, and the value
// SaveSettings interprets as "keep the existing token unchanged." Any
// other value (including the empty string) replaces the stored token.
const MCPTokenRedacted = "<set>"

func (s *AppService) GetSettings() (models.AppSettings, error) {
	settings, err := s.db.GetSettings()
	if err != nil {
		return settings, err
	}
	// Decrypt the persisted token (if any) so internal callers see the
	// plaintext. The API GET handler is the boundary that re-redacts
	// before returning to the SPA — see internal/api/handler.go.
	if settings.MCPToken != "" && secretbox.IsBlob(settings.MCPToken) {
		plain, derr := secretbox.OpenString(settings.MCPToken)
		if derr != nil {
			return settings, fmt.Errorf("decrypt mcp token: %w", derr)
		}
		settings.MCPToken = plain
	}
	return settings, nil
}

func (s *AppService) SaveSettings(settings models.AppSettings) error {
	// "<set>" is the placeholder GET returns when a token is configured —
	// when the SPA round-trips settings back unchanged we must NOT overwrite
	// the stored token with a literal "<set>". Resolve it back to whatever
	// is currently persisted.
	if settings.MCPToken == MCPTokenRedacted {
		current, err := s.db.GetSettings()
		if err != nil {
			return fmt.Errorf("read existing settings: %w", err)
		}
		if current.MCPToken != "" && secretbox.IsBlob(current.MCPToken) {
			plain, derr := secretbox.OpenString(current.MCPToken)
			if derr != nil {
				return fmt.Errorf("decrypt existing mcp token: %w", derr)
			}
			settings.MCPToken = plain
		} else {
			settings.MCPToken = current.MCPToken
		}
	}
	if err := ValidateSettings(settings); err != nil {
		return err
	}
	if settings.MCPToken != "" {
		sealed, err := secretbox.SealString(settings.MCPToken)
		if err != nil {
			return fmt.Errorf("encrypt mcp token: %w", err)
		}
		settings.MCPToken = sealed
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

func (s *AppService) GetLogsFiltered(level, search, risk string) ([]db.LogEntry, error) {
	return s.db.GetLogsFiltered(level, search, risk)
}

func (s *AppService) ClearLogs() (int64, error) {
	return s.db.ClearLogs()
}

// Log emits an audit entry without a request-scoped context. Prefer LogCtx
// when a context is in scope so the audit row can be correlated back to the
// originating HTTP request. This form remains for callbacks passed to
// external packages (scanner, firmware) that use the narrower signature.
func (s *AppService) Log(level, msg string) {
	s.logf(context.Background(), level, SanitizeLogMessage(msg))
}

// LogCtx emits an audit entry carrying the given context. The callback
// installed in the handler pulls the request ID out of ctx so the audit_log
// row and slog line link back to the originating HTTP request.
func (s *AppService) LogCtx(ctx context.Context, level, msg string) {
	s.logf(ctx, level, SanitizeLogMessage(msg))
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
	if settings.Compliance.RPCUDPPort != nil && *settings.Compliance.RPCUDPPort < 0 {
		return fmt.Errorf("rpc udp port must be 0 or greater")
	}
	if lat := settings.Compliance.Lat; lat != nil && (*lat < -90 || *lat > 90) {
		return fmt.Errorf("latitude must be between -90 and 90")
	}
	if lon := settings.Compliance.Lon; lon != nil && (*lon < -180 || *lon > 180) {
		return fmt.Errorf("longitude must be between -180 and 180")
	}
	if settings.MCPEnabled && len(settings.MCPToken) < 16 {
		return fmt.Errorf("mcp token must be at least 16 characters when MCP is enabled")
	}
	// Fail-fast on bad regex patterns in custom rules. Without this, a typo in
	// the UI would silently classify every device as "mismatch" because the
	// compile error is swallowed at evaluation time (compliance.go:checkOp).
	for i, rule := range settings.Compliance.CustomRules {
		if rule.Op != "regex" {
			continue
		}
		if _, err := regexp.Compile(rule.Value); err != nil {
			label := rule.Label
			if label == "" {
				label = fmt.Sprintf("#%d", i+1)
			}
			return fmt.Errorf("custom rule %q has invalid regex: %v", label, err)
		}
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

func DecodeSecretValue(envKey string) string {
	if value := os.Getenv(envKey + "_FILE"); value != "" {
		body, err := os.ReadFile(value)
		if err == nil {
			return strings.TrimSpace(string(body))
		}
	}
	return strings.TrimSpace(os.Getenv(envKey))
}
