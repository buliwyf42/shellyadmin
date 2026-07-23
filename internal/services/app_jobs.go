package services

import (
	"context"
	"time"

	"shellyadmin/internal/core/firmware"
	"shellyadmin/internal/models"
	"shellyadmin/internal/services/jobs"
)

// Type aliases re-export the job types from internal/services/jobs so
// existing callers (internal/mcp, internal/api, internal/services_test)
// keep using services.ScanStatus / services.FirmwareInstallStatus / etc.
// unchanged. The underlying definitions live in
// internal/services/jobs/types.go (M7 first move, see
// docs/plans/phase-4b-refactor-block.md Block 4b.1.4).
type (
	ScanStatus                = jobs.ScanStatus
	FirmwareStatus            = jobs.FirmwareStatus
	ScanJobPayload            = jobs.ScanJobPayload
	ScanJobResult             = jobs.ScanJobResult
	FirmwareJobResult         = jobs.FirmwareJobResult
	FirmwareInstallResult     = jobs.FirmwareInstallResult
	FirmwareInstallJobPayload = jobs.FirmwareInstallJobPayload
	FirmwareInstallJobResult  = jobs.FirmwareInstallJobResult
	FirmwareInstallStatus     = jobs.FirmwareInstallStatus
)

const (
	staleScanGrace                     = jobs.StaleScanGrace
	staleRefreshGrace                  = jobs.StaleRefreshGrace
	defaultFirmwareInstallTimeout      = jobs.DefaultFirmwareInstallTimeout
	defaultFirmwareInstallPollInterval = jobs.DefaultFirmwareInstallPollInterval
	firmwareInstallConcurrency         = jobs.FirmwareInstallConcurrency
)

// refreshProbeTimeout stays here so the non-job RefreshDevice path in
// app.go can compute the per-device timeout the same way the job worker
// would. Once RefreshDevice itself moves out, this can go.
func refreshProbeTimeout(settings models.AppSettings) time.Duration {
	return jobs.RefreshProbeTimeout(settings)
}

// --- Refresh ---

// RefreshDevices delegates to internal/services/jobs.Service.RefreshDevices.
// The method body moved to internal/services/jobs/refresh.go in v0.3.0
// (M7 Block 4b.1.4); this delegator preserves the public surface so the
// API handler + test callers don't see the move.
func (s *AppService) RefreshDevices(ctx context.Context) ([]models.Device, error) {
	return s.jobsSvc.RefreshDevices(ctx)
}

// --- Scan ---

// StartScan delegates to internal/services/jobs.Service.StartScan. The
// method body moved to internal/services/jobs/scan.go in v0.3.0
// (M7 Block 4b.1.4); the delegator preserves the public surface so
// existing API handlers and tests are unchanged.
func (s *AppService) StartScan() error { return s.jobsSvc.StartScan() }

// ScanStatus delegates to internal/services/jobs.Service.ScanStatus.
func (s *AppService) ScanStatus() (ScanStatus, error) { return s.jobsSvc.ScanStatus() }

// ConfirmScan delegates to internal/services/jobs.Service.ConfirmScan.
func (s *AppService) ConfirmScan(macs []string) (int, error) {
	return s.jobsSvc.ConfirmScan(macs)
}

// --- Firmware check ---

// StartFirmwareCheck delegates to jobs.Service.StartFirmwareCheck.
func (s *AppService) StartFirmwareCheck() (int, error) {
	return s.jobsSvc.StartFirmwareCheck()
}

// runFirmwareCheckScheduler delegates to jobs.Service.RunFirmwareCheckScheduler.
// Kept as the unexported method so StartBackgroundWorkers can spawn it
// unchanged.
func (s *AppService) runFirmwareCheckScheduler() {
	s.jobsSvc.RunFirmwareCheckScheduler()
}

// FirmwareStatus delegates to jobs.Service.FirmwareStatus.
func (s *AppService) FirmwareStatus() (FirmwareStatus, error) {
	return s.jobsSvc.FirmwareStatus()
}

// FirmwareUpdate delegates to jobs.Service.FirmwareUpdate.
func (s *AppService) FirmwareUpdate(ctx context.Context, macs []string, stage string) ([]firmware.UpdateResult, error) {
	return s.jobsSvc.FirmwareUpdate(ctx, macs, stage)
}

// --- Firmware install (bulk) ---

// StartFirmwareInstall is the entry point used by the bulk Update page. It
// reserves each MAC, spawns a single background job that runs Shelly.Update
// with bounded concurrency, then polls each device's Shelly.GetDeviceInfo
// at the configured poll interval until the reported version changes (or
// matches the per-channel target captured by the latest firmware_check),
// timing out at firmwareInstallTimeout.
// StartFirmwareInstall delegates to jobs.Service.StartFirmwareInstall.
func (s *AppService) StartFirmwareInstall(macs []string, stage string) (int64, int, error) {
	return s.jobsSvc.StartFirmwareInstall(macs, stage)
}

// FirmwareInstallStatus delegates to jobs.Service.FirmwareInstallStatus.
func (s *AppService) FirmwareInstallStatus() (FirmwareInstallStatus, error) {
	return s.jobsSvc.FirmwareInstallStatus()
}

// Thin wrappers forwarding to internal/services/jobs. These remain on
// AppService for callers in the wider services package (e.g.
// firmware_install_timeout SPA display in handlers, tests in
// app_jobs_test.go) that reference the unexported names.

func firmwareInstallTimeoutFromSettings(s models.AppSettings) time.Duration {
	return jobs.FirmwareInstallTimeoutFromSettings(s)
}

func firmwareInstallPollIntervalFromSettings(s models.AppSettings) time.Duration {
	return jobs.FirmwareInstallPollIntervalFromSettings(s)
}

func firmwareSchedulerDecision(now time.Time, intervalSec int, nextRun time.Time) (time.Time, bool) {
	return jobs.FirmwareSchedulerDecision(now, intervalSec, nextRun)
}

func formatTimeout(d time.Duration) string { return jobs.FormatTimeout(d) }

// --- Recovery ---

// RecoverInterruptedJobs delegates to jobs.Service.RecoverInterruptedJobs.
func (s *AppService) RecoverInterruptedJobs() error {
	return s.jobsSvc.RecoverInterruptedJobs()
}

// --- Concurrency reservations (provision/firmware mutual exclusion) ---

func (s *AppService) reserveProvisionTargets(requested []string) (allowed []string, skipped []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range requested {
		if key == "" {
			continue
		}
		if s.activeFirmware[key] {
			skipped = append(skipped, key)
			continue
		}
		s.activeProvision[key] = true
		allowed = append(allowed, key)
	}
	return allowed, skipped
}

func (s *AppService) releaseProvisionTargets(keys []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range keys {
		delete(s.activeProvision, key)
	}
}

func (s *AppService) reserveFirmwareTargets(requested []string) (allowed []string, skipped []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range requested {
		if key == "" {
			continue
		}
		if s.activeProvision[key] {
			skipped = append(skipped, key)
			continue
		}
		s.activeFirmware[key] = true
		allowed = append(allowed, key)
	}
	return allowed, skipped
}

func (s *AppService) releaseFirmwareTargets(keys []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range keys {
		delete(s.activeFirmware, key)
	}
}

// Parser entry points (ParseScanPayload/Result, ParseFirmwareResult) moved
// to the top of the file alongside the type aliases.
