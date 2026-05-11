package services

// Host-interface implementation for the internal/services/jobs sub-package.
// These thin exported wrappers expose the AppService internals (lifecycle
// context, bgJobs WaitGroup, jobSpawnMu, metrics sink, RPC client
// factories) that jobs.Service needs to drive long-running goroutines.
// See internal/services/jobs/service.go for the Host interface contract.
//
// MOVED FROM internal/services/app.go + app_clients.go — v0.3.0
// services-layer split (M7, docs/plans/phase-4b-refactor-block.md Block
// 4b.1.4). Existing unexported methods (linkedContext, scannerProbeOptions,
// firmwareOptions, refreshDeviceCapabilities, metricInc) stay so other
// services-package code calls them directly; the exported wrappers here
// satisfy jobs.Host.

import (
	"context"
	"sync"
	"time"

	"shellyadmin/internal/core/firmware"
	"shellyadmin/internal/core/scanner"
	"shellyadmin/internal/models"
)

// LinkedContext forwards to s.linkedContext (returns a context cancelled
// by either the parent or the service's shutdown context).
func (s *AppService) LinkedContext(parent context.Context) (context.Context, context.CancelFunc) {
	return s.linkedContext(parent)
}

// BackgroundJobs returns the WaitGroup that tracks in-flight job
// goroutines. Stop() drains this on shutdown.
func (s *AppService) BackgroundJobs() *sync.WaitGroup { return &s.bgJobs }

// JobSpawnMu returns the mutex that serialises the check-then-spawn race
// for duplicate-job detection.
func (s *AppService) JobSpawnMu() *sync.Mutex { return &s.jobSpawnMu }

// ShutdownContext returns the service's lifecycle context. Job workers
// select on its Done() channel to abort gracefully on Stop().
func (s *AppService) ShutdownContext() context.Context { return s.ctx }

// MetricInc forwards to s.metricInc (nil-safe).
func (s *AppService) MetricInc(name string) { s.metricInc(name) }

// ScannerProbeOptions forwards to s.scannerProbeOptions.
func (s *AppService) ScannerProbeOptions(d models.Device, timeout time.Duration) scanner.ProbeOptions {
	return s.scannerProbeOptions(d, timeout)
}

// FirmwareOptions forwards to s.firmwareOptions.
func (s *AppService) FirmwareOptions(d models.Device, timeout time.Duration) firmware.Options {
	return s.firmwareOptions(d, timeout)
}

// RefreshDeviceCapabilities forwards to s.refreshDeviceCapabilities.
func (s *AppService) RefreshDeviceCapabilities(ctx context.Context, d *models.Device) {
	s.refreshDeviceCapabilities(ctx, d)
}

// ValidateSettings forwards to the services-level ValidateSettings
// function so jobs.Host can gate StartScan on a normalize-then-validate
// pass without importing services (cycle).
func (s *AppService) ValidateSettings(settings models.AppSettings) error {
	return ValidateSettings(settings)
}

// ReserveFirmwareTargets / ReleaseFirmwareTargets forward to the
// unexported helpers. The activeProvision/activeFirmware maps live on
// AppService so a provision and a firmware op against the same target
// cannot run concurrently; jobs.Host gives jobs.Service the same surface.
func (s *AppService) ReserveFirmwareTargets(requested []string) (allowed []string, skipped []string) {
	return s.reserveFirmwareTargets(requested)
}

func (s *AppService) ReleaseFirmwareTargets(keys []string) {
	s.releaseFirmwareTargets(keys)
}
