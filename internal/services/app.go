package services

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"shellyadmin/internal/core/compliance"
	"shellyadmin/internal/db"
	"shellyadmin/internal/models"
	"shellyadmin/internal/services/audit"
	"shellyadmin/internal/services/backup"
	"shellyadmin/internal/services/credentials"
	"shellyadmin/internal/services/jobs"
	"shellyadmin/internal/services/loginlock"
	"shellyadmin/internal/services/logs"
	"shellyadmin/internal/services/provisioning"
	"shellyadmin/internal/services/sessions"
	servicessettings "shellyadmin/internal/services/settings"
	"shellyadmin/internal/services/templates"
	"shellyadmin/internal/services/tokens"
	"shellyadmin/internal/services/totp"
	"shellyadmin/internal/services/validation"
	"shellyadmin/internal/services/workers"
	"shellyadmin/internal/util"
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

	// authMu serialises the check-then-write of the admin credential so two
	// concurrent setup POSTs within a process cannot both pass the
	// "not configured yet" guard. Cross-process is already covered by the
	// single-instance runtime lock (ADR-0015).
	authMu sync.Mutex

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

	// templates owns template-table CRUD with credential_ref + size +
	// JSON-shape validation. Extracted to internal/services/templates in
	// v0.3.0 (M7).
	templates *templates.Service

	// logs is a thin pass-through over the audit_log table CRUD. Extracted
	// to internal/services/logs in v0.3.0 (M7).
	logs *logs.Service

	// audit owns the service-level audit-log sink (sanitize + emit +
	// optional webhook forward). Extracted to internal/services/audit in
	// v0.3.0 (M7).
	audit *audit.Service

	// settings owns the encrypted-at-rest MCP-token envelope handling +
	// the validate-before-save pipeline. Extracted to
	// internal/services/settings in v0.3.0 (M7); GetSettings + SaveSettings
	// delegate here.
	settings *servicessettings.Service

	// totp owns the TOTP 2FA orchestration (enrollment / verify / disable /
	// login-verify). T1 in v0.3.0 (docs/plans/phase-4c-auth-strategics.md,
	// Block 4c.1); the *AppService delegators in internal/services/totp.go
	// preserve the public surface so api/handler_totp.go stays free of the
	// totp sub-package import.
	totp *totp.Service

	// tokens owns the Personal Access Token lifecycle (create / list /
	// revoke / middleware lookup). T3 in v0.3.0 (Block 4c.2); the
	// *AppService delegators in internal/services/tokens.go preserve the
	// public surface for the api package + the auth middleware.
	tokens *tokens.Service

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

// TemplateRecord is re-exported from internal/services/templates so
// existing api/handler_templates.go references compile unchanged.
type TemplateRecord = templates.Record

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
	// templates + logs are thin pass-throughs over Store; constructed
	// here for symmetry with the rest of the sub-service tree.
	svc.templates = templates.New(database)
	svc.logs = logs.New(database)
	// audit.Service takes the existing logf callback so persisted rows
	// flow through db.AddLog unchanged. svc itself satisfies audit.MetricSink
	// via IncLabelled (forwarded to the nil-safe s.metricIncLabelled).
	svc.audit = audit.New(database, logf, svc)
	// settings.Service wraps the secretbox encrypt/decrypt + validate
	// pipeline. The onSaved callback fires ReconcileMCPFromSettings so a
	// token rotation rebuilds the live MCP listener.
	svc.settings = servicessettings.New(database, svc.ReconcileMCPFromSettings)
	// totp.Service handles the per-operator enrollment + verify flows.
	// Store-only dependency; secretbox key is read at the package level
	// during seal/open so no key wiring is needed here.
	svc.totp = totp.New(database)
	// tokens.Service handles Personal Access Token lifecycle. Pure
	// Store-only deps; the bearer-string hash comparison is done in-
	// package via sha256+ConstantTimeCompare.
	svc.tokens = tokens.New(database)
	return svc
}

// IncLabelled satisfies audit.MetricSink for the audit-sink wiring above.
// Forwards to the nil-safe s.metricIncLabelled so a nil metrics sink at
// construction time is harmless (callers use SetMetrics later).
func (s *AppService) IncLabelled(name string, labels map[string]string) {
	s.metricIncLabelled(name, labels)
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

// RefreshDevice delegates to jobs.Service.RefreshDevice. The single-device
// refresh path moved to internal/services/jobs/refresh.go alongside the
// fleet-wide RefreshDevices in v0.3.0 (M7 Block 4b.1).
func (s *AppService) RefreshDevice(ctx context.Context, target string) ([]models.Device, error) {
	return s.jobsSvc.RefreshDevice(ctx, target)
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

// checkAuthRequired moved with its caller (the single-device RefreshDevice
// path) to internal/services/jobs/service.go. Removed here in v0.3.0.

// MCP token validation now happens entirely inside
// internal/services/validation.Settings; the local mcpTokenPattern alias
// dropped along with the inline pattern match in v0.3.0 (M7).

// GetSettings delegates to internal/services/settings.Service.Get.
func (s *AppService) GetSettings() (models.AppSettings, error) {
	return s.settings.Get()
}

// SaveSettings delegates to internal/services/settings.Service.Save.
func (s *AppService) SaveSettings(settings models.AppSettings) error {
	return s.settings.Save(settings)
}

// Templates CRUD delegates to internal/services/templates.Service.
func (s *AppService) ListTemplates() ([]string, error) { return s.templates.List() }
func (s *AppService) GetTemplate(name string) (TemplateRecord, error) {
	return s.templates.Get(name)
}
func (s *AppService) SaveTemplate(name, content, credentialRef string) error {
	return s.templates.Save(name, content, credentialRef)
}
func (s *AppService) DeleteTemplate(name string) error { return s.templates.Delete(name) }

// Logs read + clear delegates to internal/services/logs.Service.
func (s *AppService) GetLogs(level, search string) ([]db.LogEntry, error) {
	return s.logs.Get(level, search)
}
func (s *AppService) GetLogsFiltered(level, search, risk string) ([]db.LogEntry, error) {
	return s.logs.GetFiltered(level, search, risk)
}
func (s *AppService) ClearLogs() (int64, error) { return s.logs.Clear() }

// Log delegates to audit.Service.Log.
func (s *AppService) Log(level, msg string) { s.audit.Log(level, msg) }

// LogCtx delegates to audit.Service.LogCtx.
func (s *AppService) LogCtx(ctx context.Context, level, msg string) {
	s.audit.LogCtx(ctx, level, msg)
}

// ValidateSettings delegates to internal/services/validation.Settings.
func ValidateSettings(settings models.AppSettings) error {
	return validation.Settings(settings)
}

// ValidateTemplate delegates to internal/services/validation.Template.
func ValidateTemplate(template map[string]interface{}) error {
	return validation.Template(template)
}

// SanitizeLogMessage delegates to internal/services/audit.SanitizeLogMessage.
// External callers (cmd/shellyctl/main.go, internal/mcp, internal/api) keep
// importing services.SanitizeLogMessage; the implementation lives in audit.
func SanitizeLogMessage(msg string) string { return audit.SanitizeLogMessage(msg) }

// DecodeSecretValue delegates to internal/util.DecodeSecretValue.
// cmd/shellyctl/main.go imports services for several things at boot;
// keeping this re-export means main.go doesn't need a separate util
// import just for the four secret reads.
func DecodeSecretValue(envKey string) string { return util.DecodeSecretValue(envKey) }

// BoundedConcurrency moved to internal/services/jobs (the only caller).
// Removed here as dead code in v0.3.0 (M7).
