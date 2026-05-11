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
