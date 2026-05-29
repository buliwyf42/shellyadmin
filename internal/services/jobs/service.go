// Service-level scaffolding for the jobs sub-package. AppService keeps a
// *Service field; the Service.RefreshDevices / Service.StartScan / etc.
// methods are the new home for the long-running goroutines moved out of
// internal/services/app_jobs.go.
//
// MOVED FROM internal/services/app_jobs.go — v0.3.0 services-layer split
// (M7, docs/plans/phase-4b-refactor-block.md Block 4b.1.4). The Store +
// Host split is deliberate: Store is the raw DB row surface (jobs uses
// db.GetSettings / db.SaveSettings, NOT AppService's secretbox-wrapped
// versions), while Host is the runtime/concurrency surface AppService
// itself implements (linkedContext, bgJobs, mu helpers, RPC factories).

package jobs

import (
	"context"
	"net/http"
	"sync"
	"time"

	"shellyadmin/internal/core/firmware"
	"shellyadmin/internal/core/scanner"
	"shellyadmin/internal/models"
)

// Store is the narrow persistence surface jobs needs. *db.DB satisfies it
// structurally; tests can substitute a fake. NOTE: GetSettings and
// SaveSettings here are the RAW DB row methods — they deliberately match
// *db.DB and NOT *AppService (which wraps secretbox encrypt/decrypt around
// the MCP token). Job code wants the raw row.
type Store interface {
	ListDevices() ([]models.Device, error)
	UpsertDevice(d models.Device) error
	UpsertDevices([]models.Device) error
	GetSettings() (models.AppSettings, error)
	SaveSettings(s models.AppSettings) error

	CreateJob(jobType, restartPolicy, payload string, total int) (int64, error)
	UpdateJobProgress(id int64, done, total int, result string) error
	IncrementJobDone(id int64) error
	CompleteJob(id int64, status, result, errText string, done, total int) error
	InterruptJob(id int64, errText string) error
	GetLatestJob(jobType string) (models.Job, error)
	GetJob(id int64) (models.Job, error)
	ListInterruptedRestartableJobs() ([]models.Job, error)
}

// Host is the runtime/concurrency + RPC-factory surface jobs needs. The
// implementing type (typically *AppService) owns the lifecycle context,
// the background-goroutine WaitGroup, the job-spawn mutex, and the
// per-device credential resolution that lives on top of the DB.
type Host interface {
	// LinkedContext returns a context that cancels when either parent or
	// the host's shutdown context cancels.
	LinkedContext(parent context.Context) (context.Context, context.CancelFunc)

	// BackgroundJobs returns the host's bgJobs WaitGroup. The job goroutine
	// must call Add(1) before starting and Done() at exit so the host's
	// Stop() path can drain.
	BackgroundJobs() *sync.WaitGroup

	// JobSpawnMu returns the host's job-spawn mutex (serializes the
	// check-then-spawn race so two concurrent triggers can't both pass
	// the "already running" gate).
	JobSpawnMu() *sync.Mutex

	// ShutdownContext is the host's lifecycle context. Job workers select
	// on its Done() channel to abort gracefully on Stop().
	ShutdownContext() context.Context

	// Log / LogCtx forward to the host's audit-log writer.
	Log(level, msg string)
	LogCtx(ctx context.Context, level, msg string)

	// MetricInc forwards to the host's metrics sink (nil-safe on the host).
	MetricInc(name string)

	// ScannerProbeOptions / FirmwareOptions build per-device RPC client
	// options from the host's credential + TLS state.
	ScannerProbeOptions(d models.Device, timeout time.Duration) scanner.ProbeOptions
	FirmwareOptions(d models.Device, timeout time.Duration) firmware.Options

	// RefreshDeviceCapabilities is called in-place on each probed device
	// to repopulate firmware availability + ListMethods cache.
	RefreshDeviceCapabilities(ctx context.Context, d *models.Device)

	// GetDevices returns the post-compliance-evaluation device list (the
	// SPA-visible shape). RefreshDevices returns this to the API handler
	// once the worker goroutine completes.
	GetDevices() ([]models.Device, error)

	// ValidateScanParams runs the services-level normalize-then-validate
	// pipeline on the scan-relevant fields of a settings row and returns the
	// total subnet+mDNS target count. StartScan gates on this so a
	// misconfigured row (out-of-range timeouts / subnet count) doesn't spawn a
	// job at all. It ignores the row's MCP token — which jobs receives as
	// secretbox ciphertext — so the URL-safe-alphabet check never trips here.
	ValidateScanParams(s models.AppSettings) (int, error)

	// ReserveFirmwareTargets / ReleaseFirmwareTargets mutex per-device
	// firmware operations against in-flight provisions. Keys are
	// "ip:<ip>" or "mac:<mac>"; the host maintains the underlying
	// activeProvision/activeFirmware maps so a provision and a firmware
	// update can't run against the same device simultaneously.
	ReserveFirmwareTargets(requested []string) (allowed []string, skipped []string)
	ReleaseFirmwareTargets(keys []string)
}

// Service hosts the job-orchestration methods.
type Service struct {
	store Store
	host  Host
}

// New constructs a Service backed by the given Store and Host.
func New(store Store, host Host) *Service {
	return &Service{store: store, host: host}
}

// boundedConcurrency clamps a configured concurrency value into a sane
// range. Duplicated from services.BoundedConcurrency because the sub-package
// can't import services without creating a cycle.
func boundedConcurrency(value int) int {
	switch {
	case value <= 0:
		return 32
	case value > 128:
		return 128
	default:
		return value
	}
}

// checkAuthRequired probes a device's /shelly endpoint over plain HTTP and
// reports whether the device answered 401/403 (auth required). Duplicated
// from services.checkAuthRequired for the same reason as boundedConcurrency.
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
