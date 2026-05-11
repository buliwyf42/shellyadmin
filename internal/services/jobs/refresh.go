// MOVED FROM internal/services/app_jobs.go — v0.3.0 services-layer split
// (M7, docs/plans/phase-4b-refactor-block.md Block 4b.1.4). RefreshDevices
// and runRefreshJob lift verbatim onto *Service with their AppService
// receiver references rewritten as s.store.* (DB) and s.host.* (lifecycle,
// metrics, RPC factories). Behaviour is identical — the unit-test surface
// in app_jobs_test.go continues to pass via the services delegator.

package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"shellyadmin/internal/core/scanner"
	"shellyadmin/internal/models"
)

// RefreshDevice re-probes a single device matched by MAC, IP, or name
// and returns the post-refresh device list. Used by the per-row "refresh"
// action on the Devices page; the bulk refresh goes through
// RefreshDevices instead. The auth-required / TLS-cert-invalid /
// rate-limited paths persist the failure on the existing row and keep
// the rich fields (FirstSeen, DeviceNum, firmware cache) intact.
func (s *Service) RefreshDevice(ctx context.Context, target string) ([]models.Device, error) {
	devices, err := s.store.ListDevices()
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

	settings, err := s.store.GetSettings()
	if err != nil {
		return nil, err
	}
	timeout := RefreshProbeTimeout(settings)
	attemptedAt := time.Now().UTC().Format(time.RFC3339)
	opts := s.host.ScannerProbeOptions(*current, timeout)
	probed := scanner.ProbeDeviceWithOptions(ctx, current.IP, opts, s.host.Log)
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
		if err := s.store.UpsertDevice(*current); err != nil {
			return nil, err
		}
		return s.host.GetDevices()
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
		if err := s.store.UpsertDevice(*current); err != nil {
			return nil, err
		}
		return s.host.GetDevices()
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
	s.host.RefreshDeviceCapabilities(ctx, probed)
	if err := s.store.UpsertDevice(*probed); err != nil {
		return nil, err
	}
	return s.host.GetDevices()
}

// RefreshDevices spawns a refresh job that re-probes every persisted
// device. Blocks until the worker goroutine signals completion, then
// returns the post-refresh device list (with compliance + counts).
//
// S10: serialises the check-then-spawn against the host's JobSpawnMu so
// two concurrent requests cannot both observe "no job running" between
// GetLatestJob and CreateJob.
func (s *Service) RefreshDevices(ctx context.Context) ([]models.Device, error) {
	spawnMu := s.host.JobSpawnMu()
	spawnMu.Lock()
	defer spawnMu.Unlock()
	if latest, err := s.store.GetLatestJob("refresh"); err == nil && latest.Status == "running" {
		stale, staleErr := RefreshJobStale(latest, time.Now())
		if staleErr == nil && stale {
			if ierr := s.store.InterruptJob(latest.ID, "refresh stalled"); ierr != nil {
				s.host.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: mark stalled failed: %v", latest.ID, ierr))
			}
		} else {
			return nil, errors.New("refresh already running")
		}
	}
	jobID, err := s.store.CreateJob("refresh", "auto", "{}", 0)
	if err != nil {
		return nil, err
	}
	s.host.MetricInc("shellyadmin_refresh_jobs_total")
	jobCtx, cancel := s.host.LinkedContext(ctx)
	done := make(chan error, 1)
	bg := s.host.BackgroundJobs()
	bg.Add(1)
	go func() {
		defer cancel()
		defer bg.Done()
		s.runRefreshJob(jobCtx, jobID, done)
	}()
	if err := <-done; err != nil {
		return nil, err
	}
	return s.host.GetDevices()
}

func (s *Service) runRefreshJob(ctx context.Context, jobID int64, done chan<- error) {
	devices, err := s.store.ListDevices()
	if err != nil {
		if cerr := s.store.CompleteJob(jobID, "failed", "", err.Error(), 0, 0); cerr != nil {
			s.host.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: complete-failed write failed: %v", jobID, cerr))
		}
		done <- err
		return
	}
	settings, err := s.store.GetSettings()
	if err != nil {
		if cerr := s.store.CompleteJob(jobID, "failed", "", err.Error(), 0, 0); cerr != nil {
			s.host.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: complete-failed write failed: %v", jobID, cerr))
		}
		done <- err
		return
	}
	timeout := RefreshProbeTimeout(settings)
	limit := boundedConcurrency(settings.ScanConcurrency)
	if limit > len(devices) {
		limit = len(devices)
	}
	if limit < 1 {
		limit = 1
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	refreshed := make([]models.Device, 0, len(devices))
	work := make(chan models.Device)
	if perr := s.store.UpdateJobProgress(jobID, 0, len(devices), ""); perr != nil {
		s.host.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: initial progress update failed: %v", jobID, perr))
	}

	for i := 0; i < limit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for device := range work {
				select {
				case <-ctx.Done():
					if ierr := s.store.IncrementJobDone(jobID); ierr != nil {
						s.host.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: increment done failed: %v", jobID, ierr))
					}
					return
				default:
				}
				attemptedAt := time.Now().UTC().Format(time.RFC3339)
				updated := device
				probeOpts := s.host.ScannerProbeOptions(device, timeout)
				if found := scanner.ProbeDeviceWithOptions(ctx, device.IP, probeOpts, s.host.Log); found != nil && !found.AuthRequired {
					found.DeviceNum = device.DeviceNum
					found.FirstSeen = device.FirstSeen
					found.LastRefreshAttempt = attemptedAt
					found.LastRefreshOK = true
					found.LastRefreshError = ""
					found.ConsecutiveMisses = 0
					found.Online = true
					found.AuthRequired = false
					found.AuthError = ""
					found.AuthLockedUntil = ""
					found.TLSAllowInsecure = device.TLSAllowInsecure
					// Preserve firmware cache (scanner doesn't repopulate
					// these); RefreshDeviceCapabilities below overwrites on success.
					found.FWAvailableStable = device.FWAvailableStable
					found.FWAvailableBeta = device.FWAvailableBeta
					found.FWCheckedAt = device.FWCheckedAt
					found.FWAutoUpdate = device.FWAutoUpdate
					s.host.RefreshDeviceCapabilities(ctx, found)
					updated = *found
				} else if found != nil && found.AuthRequired {
					updated.LastRefreshAttempt = attemptedAt
					updated.LastRefreshOK = false
					updated.AuthRequired = true
					updated.AuthError = found.AuthError
					if found.AuthLockedUntil != "" {
						updated.AuthLockedUntil = found.AuthLockedUntil
					}
					updated.LastRefreshError = found.AuthError
					updated.Online = true
					updated.ConsecutiveMisses = 0
				} else {
					updated.LastRefreshAttempt = attemptedAt
					updated.LastRefreshOK = false
					if required, reason := checkAuthRequired(ctx, device.IP, timeout); required {
						updated.AuthRequired = true
						updated.AuthError = reason
						updated.LastRefreshError = reason
						updated.Online = true
						updated.ConsecutiveMisses = 0
					} else {
						updated.LastRefreshError = "refresh timed out"
						updated.ConsecutiveMisses++
						if updated.ConsecutiveMisses >= 2 {
							updated.Online = false
						}
					}
				}
				mu.Lock()
				refreshed = append(refreshed, updated)
				mu.Unlock()
				if ierr := s.store.IncrementJobDone(jobID); ierr != nil {
					s.host.Log("error", fmt.Sprintf("refresh job %d: increment done failed: %v", jobID, ierr))
				}
			}
		}()
	}
	for _, device := range devices {
		work <- device
	}
	close(work)
	wg.Wait()
	if ctx.Err() != nil {
		if ierr := s.store.InterruptJob(jobID, "service shutdown"); ierr != nil {
			s.host.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: mark interrupted on shutdown: %v", jobID, ierr))
		}
		done <- ctx.Err()
		return
	}
	if err := s.store.UpsertDevices(refreshed); err != nil {
		if cerr := s.store.CompleteJob(jobID, "failed", "", err.Error(), len(devices), len(devices)); cerr != nil {
			s.host.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: complete-failed write failed: %v", jobID, cerr))
		}
		done <- err
		return
	}
	body, err := json.Marshal(map[string]any{"refreshed": len(refreshed)})
	if err != nil {
		s.host.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: marshal result body failed: %v", jobID, err))
	}
	if cerr := s.store.CompleteJob(jobID, "completed", string(body), "", len(devices), len(devices)); cerr != nil {
		s.host.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: complete-success write failed: %v", jobID, cerr))
	}
	done <- nil
}
