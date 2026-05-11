package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"shellyadmin/internal/core/compliance"
	"shellyadmin/internal/core/scanner"
	"shellyadmin/internal/core/secretbox"
	"shellyadmin/internal/db"
	"shellyadmin/internal/middleware"
	"shellyadmin/internal/models"
	"shellyadmin/internal/services/backup"
	"shellyadmin/internal/services/credentials"
	"shellyadmin/internal/services/jobs"
	"shellyadmin/internal/services/loginlock"
	"shellyadmin/internal/services/provisioning"
	"shellyadmin/internal/services/sessions"
	"shellyadmin/internal/services/validation"
	"shellyadmin/internal/services/workers"
)

const (
	// MaxJSONBytes is the per-request JSON payload cap on the API
	// boundary. Kept services-side because handlers reference it.
	MaxJSONBytes    = 256 * 1024
	maxProvisionIPs = 256
)

// Backward-compat re-exports. Definitions live in
// internal/services/validation; services.MaxTemplateBytes etc. stay
// importable so internal/api and tests don't need touching.
const (
	MaxTemplateBytes = validation.MaxTemplateBytes
	maxScanTargets   = validation.MaxScanTargets
	MCPTokenRedacted = validation.MCPTokenRedacted
)

type AppService struct {
	db      Store
	logf    func(ctx context.Context, level, msg string)
	dataDir string

	mu              sync.Mutex
	activeProvision map[string]bool
	activeFirmware  map[string]bool

	// jobSpawnMu serialises the "is a job of this type still running" check
	// against the actual spawn so two concurrent requests cannot both
	// observe "no job running" between GetLatestJob and CreateJob. Each
	// job-type uses a separate goroutine-safe path under this lock.
	// S10 closes this race; before, two near-simultaneous /api/refresh
	// requests could both create a refresh job (the second would notice
	// during the next stale check, but two were already counted in the
	// jobs table).
	jobSpawnMu sync.Mutex

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

	// mcp owns the live MCP listener lifecycle when SetMCPParams has
	// been called. nil for tests / callers that never wire MCP up;
	// see internal/services/app_mcp.go for the controller type.
	mcp *MCPController

	// sessions owns server-side session row lifecycle (issue, revoke,
	// validate). Extracted to internal/services/sessions in v0.3.0 (M7);
	// delegators on AppService preserve the public surface — see
	// internal/services/sessions.go.
	sessions *sessions.Service

	// creds owns credential + credential-group + per-device-assignment
	// CRUD. Extracted to internal/services/credentials in v0.3.0 (M7);
	// delegators on AppService preserve the public surface — see
	// internal/services/app_credentials.go.
	creds *credentials.Service

	// jobsSvc owns the long-running job orchestration (refresh / scan /
	// firmware_check / firmware_install). Extracted to internal/services/jobs
	// in v0.3.0 (M7); methods are migrated job-family at a time and
	// AppService keeps a delegator per migrated method. See
	// internal/services/app_jobs.go.
	jobsSvc *jobs.Service

	// backup owns operator-driven configuration export/import. Extracted to
	// internal/services/backup in v0.3.0 (M7); delegators on
	// AppService.ExportBackup / ImportBackup preserve the public surface —
	// see internal/services/app_backup.go.
	backup *backup.Service

	// lock owns the per-account login-failure counter + lockout window
	// (Q20). Extracted to internal/services/loginlock in v0.3.0 (M7);
	// delegators preserve the IsAccountLocked / RecordLoginFailure /
	// RecordLoginSuccess surface.
	lock *loginlock.Service

	// workers owns the long-lived background goroutines (session sweep,
	// audit retention, auto-backup, firmware-check scheduler). Extracted
	// to internal/services/workers in v0.3.0 (M7); StartBackgroundWorkers
	// delegates here.
	workers *workers.Service

	// provisioning owns the template provision + user-CA upload flows
	// (multi-device Shelly RPC). Extracted to internal/services/provisioning
	// in v0.3.0 (M7); AppService keeps delegators on Provision / UploadUserCA.
	provisioning *provisioning.Service

	// metrics is the Prometheus-format counter/gauge registry. nil for
	// callers that don't wire it up (tests, MCP-only stdio mode); the
	// service-layer Inc/Set helpers tolerate nil so the metrics path is
	// strictly additive.
	metrics MetricsSink
}

// MetricsSink is the narrow interface AppService uses to record
// observability events. *observability.Registry implements it; tests
// that need to assert metric writes can swap in a recording fake.
type MetricsSink interface {
	Inc(name string)
	IncLabelled(name string, labels map[string]string)
	Set(name string, value int64)
}

// SetMetrics wires a metrics sink into the service. Safe to call once
// during startup before any background worker is spawned.
func (s *AppService) SetMetrics(m MetricsSink) {
	s.metrics = m
}

// metricInc is the nil-safe convenience used by service-layer code.
// Callers don't need to check whether metrics is configured.
func (s *AppService) metricInc(name string) {
	if s.metrics != nil {
		s.metrics.Inc(name)
	}
}

func (s *AppService) metricSet(name string, v int64) {
	if s.metrics != nil {
		s.metrics.Set(name, v)
	}
}

func (s *AppService) metricIncLabelled(name string, labels map[string]string) {
	if s.metrics != nil {
		s.metrics.IncLabelled(name, labels)
	}
}

type TemplateRecord struct {
	Name          string `json:"name"`
	Content       string `json:"content"`
	CredentialRef string `json:"credential_ref"`
}

func NewAppService(database Store, dataDir string, logf func(ctx context.Context, level, msg string)) *AppService {
	ctx, cancel := context.WithCancel(context.Background())
	svc := &AppService{
		db:              database,
		dataDir:         dataDir,
		logf:            logf,
		activeProvision: map[string]bool{},
		activeFirmware:  map[string]bool{},
		ctx:             ctx,
		cancel:          cancel,
		sessions:        sessions.New(database),
		creds:           credentials.New(database),
		lock:            loginlock.New(database),
	}
	// jobs.Service needs two halves: Store (raw DB) and Host (AppService
	// itself, supplying lifecycle + RPC factories). Wire after the rest of
	// svc is built so the Host pointer is non-nil.
	svc.jobsSvc = jobs.New(database, svc)
	// backup.Service depends on credentials.Service.SaveGroup for the
	// admin-mirror credential write. Constructed after creds so the
	// GroupSaver pointer is non-nil.
	svc.backup = backup.New(database, svc.creds, svc.Log)
	// workers.Service runs the long-lived background loops. Spawn happens
	// later in StartBackgroundWorkers (called from main.go after
	// RecoverInterruptedJobs); construction here just wires up the deps
	// so the controller's ctx/bgJobs pointers stay live.
	svc.workers = workers.New(database, ctx, &svc.bgJobs, logf, dataDir, svc.runFirmwareCheckScheduler)
	// provisioning.Service needs the *AppService Host so it can reach the
	// reservation maps + ProvisionOptions; constructed last so the Host
	// pointer is non-nil at call time.
	svc.provisioning = provisioning.New(database, svc)
	return svc
}

// Backward-compat re-exports — implementations live in
// internal/services/loginlock.
const (
	LoginMaxFailures = loginlock.MaxFailures
	LoginLockoutDur  = loginlock.LockoutDur
)

// IsAccountLocked delegates to loginlock.Service.IsLocked.
func (s *AppService) IsAccountLocked(username string) (bool, time.Time) {
	return s.lock.IsLocked(username)
}

// RecordLoginFailure delegates to loginlock.Service.RecordFailure.
func (s *AppService) RecordLoginFailure(username string) error {
	return s.lock.RecordFailure(username)
}

// RecordLoginSuccess delegates to loginlock.Service.RecordSuccess.
func (s *AppService) RecordLoginSuccess(username string) error {
	return s.lock.RecordSuccess(username)
}

// StartBackgroundWorkers delegates to workers.Service.Start. The four
// long-lived worker goroutines (session sweeper, audit retention pruner,
// auto-backup snapshotter, firmware-check scheduler-with-panic-recover)
// moved to internal/services/workers in v0.3.0 (M7,
// docs/plans/phase-4b-refactor-block.md Block 4b.1).
func (s *AppService) StartBackgroundWorkers() {
	s.workers.Start()
}

// Stop signals background jobs to exit, waits for them to drain (bounded by
// shutdownCtx), and marks any jobs still "running" as "interrupted". Safe to
// call once; subsequent calls are no-ops.
func (s *AppService) Stop(shutdownCtx context.Context) {
	s.stopOnce.Do(func() {
		// Shut the MCP listener down first — it's an externally-visible
		// surface and we'd rather drop new MCP requests at the listener
		// than have them race the background-job drain below.
		s.stopMCP(shutdownCtx)

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
	// M4 — refresh the inventory-size gauge on every read. GetDevices
	// is called frequently enough by the SPA (every /api/devices)
	// that this stays sub-second-fresh without a dedicated ticker.
	s.metricSet("shellyadmin_devices_total", int64(len(devices)))
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
		if devices[i].MAC == target || devices[i].IP == target || devices[i].Name == target {
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

// Provision delegates to internal/services/provisioning.Service.Provision.
// The body moved out in v0.3.0 (M7 Block 4b.1); the delegator preserves
// the public surface (api/handler_provision.go, services tests).
func (s *AppService) Provision(ctx context.Context, ips []string, template map[string]interface{}, credentialRef string) ([]map[string]any, error) {
	return s.provisioning.Provision(ctx, ips, template, credentialRef)
}

// UploadUserCA delegates to internal/services/provisioning.Service.UploadUserCA.
func (s *AppService) UploadUserCA(ctx context.Context, ips []string, kind string, pem string) ([]UploadUserCAResult, error) {
	return s.provisioning.UploadUserCA(ctx, ips, kind, pem)
}

// Backward-compat re-exports — definitions live in
// internal/services/provisioning.
const MaxUserCABytes = provisioning.MaxUserCABytes

type UploadUserCAResult = provisioning.UploadUserCAResult

// isProvisionTargetAllowed is the thin shim other services-package code
// (RefreshDevice's auth-fail path) still calls. The real implementation
// lives in provisioning.IsTargetAllowed.
func isProvisionTargetAllowed(addr netip.Addr) bool {
	return provisioning.IsTargetAllowed(addr)
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

// MCP token validation now happens entirely inside
// internal/services/validation.Settings; the local mcpTokenPattern alias
// dropped along with the inline pattern match in v0.3.0 (M7).

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
	if err := s.db.SaveSettings(settings); err != nil {
		return err
	}
	// Reconcile the live MCP listener to match the new settings.
	// No-op when env-locked or when SetMCPParams was never called
	// (e.g. unit tests that don't exercise MCP).
	s.ReconcileMCPFromSettings()
	return nil
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
	s.metricIncLabelled("shellyadmin_audit_rows_written_total", map[string]string{"level": strings.ToUpper(strings.TrimSpace(level))})
	sanitized := SanitizeLogMessage(msg)
	s.logf(context.Background(), level, sanitized)
	s.maybeForwardAudit(context.Background(), level, sanitized)
}

// LogCtx emits an audit entry carrying the given context. The callback
// installed in the handler pulls the request ID out of ctx so the audit_log
// row and slog line link back to the originating HTTP request.
func (s *AppService) LogCtx(ctx context.Context, level, msg string) {
	s.metricIncLabelled("shellyadmin_audit_rows_written_total", map[string]string{"level": strings.ToUpper(strings.TrimSpace(level))})
	sanitized := SanitizeLogMessage(msg)
	s.logf(ctx, level, sanitized)
	s.maybeForwardAudit(ctx, level, sanitized)
}

// maybeForwardAudit shells out to the audit webhook delivery code if
// the operator configured one. Best-effort: errors are swallowed (the
// local audit_log row is the source of truth; the webhook is a
// replica). Reads settings on every call because the webhook URL can
// change at runtime via /api/settings — the operator should not have
// to restart the service to disable a forwarder.
func (s *AppService) maybeForwardAudit(ctx context.Context, level, msg string) {
	if s.db == nil {
		return
	}
	settings, err := s.db.GetSettings()
	if err != nil || settings.AuditWebhookURL == "" {
		return
	}
	reqID := ""
	risk := ""
	if ctx != nil {
		reqID = middleware.FromContext(ctx)
		risk = RiskFromContext(ctx)
	}
	s.forwardAudit(level, msg, reqID, risk, settings)
}

// sanitizeTags moved with its callers to internal/services/credentials and
// internal/services/backup (each holds a private copy). The function is no
// longer needed in this file.

// ValidateSettings delegates to internal/services/validation.Settings.
func ValidateSettings(settings models.AppSettings) error {
	return validation.Settings(settings)
}

// ValidateTemplate delegates to internal/services/validation.Template.
func ValidateTemplate(template map[string]interface{}) error {
	return validation.Template(template)
}

// secretPattern matches the three forms credentials appear in our log
// pipeline: `password=plain`, `password: plain`, and JSON-quoted
// `"password":"plain"`. The optional `["']?` after the key handles the
// JSON case where the quote follows the field name. S21 added regression
// tests in sanitize_log_test.go — extending the keyword set requires a
// matching test case there.
var secretPattern = regexp.MustCompile(`(?i)(password|pass|secret|ha1)["']?\s*[:=]\s*("[^"]*"|[^,\s\}\)&]+)`)

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
