// MOVED FROM internal/services/app_jobs.go — v0.3.0 services-layer split
// (M7, docs/plans/phase-4b-refactor-block.md Block 4b.1.4). The periodic
// firmware-check job (StartFirmwareCheck, runFirmwareJob, RunFirmwareJob
// recovery hook, runFirmwareCheckScheduler, FirmwareStatus) and the
// per-device update trigger (FirmwareUpdate) move onto *Service using the
// host's RPC-options factory and reservation hooks.

package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"shellyadmin/internal/core/firmware"
	"shellyadmin/internal/models"
)

// StartFirmwareCheck spawns a periodic firmware_check job that probes
// every persisted device and caches the per-channel availability +
// auto-update mode + ListMethods set. Returns the device count the
// caller can use for the progress UI.
func (s *Service) StartFirmwareCheck() (int, error) {
	if latest, err := s.store.GetLatestJob("firmware_check"); err == nil && latest.Status == "running" {
		return latest.Total, errors.New("firmware check already running")
	}
	devices, err := s.store.ListDevices()
	if err != nil {
		return 0, err
	}
	jobID, err := s.store.CreateJob("firmware_check", "auto", "{}", len(devices))
	if err != nil {
		return 0, err
	}
	s.host.MetricInc("shellyadmin_firmware_jobs_total")
	bg := s.host.BackgroundJobs()
	bg.Add(1)
	go func() {
		defer bg.Done()
		s.runFirmwareJob(jobID, devices)
	}()
	return len(devices), nil
}

// RunFirmwareJob is the exported worker entry point. Used by
// StartFirmwareCheck and the recovery path (services.RecoverInterruptedJobs
// auto-restarts interrupted firmware_check jobs by calling this with the
// device list).
func (s *Service) RunFirmwareJob(jobID int64, devices []models.Device) {
	s.runFirmwareJob(jobID, devices)
}

func (s *Service) runFirmwareJob(jobID int64, devices []models.Device) {
	shutdown := s.host.ShutdownContext()
	results := make([]firmware.Result, 0, len(devices))
	for _, device := range devices {
		if shutdown.Err() != nil {
			if ierr := s.store.InterruptJob(jobID, "service shutdown"); ierr != nil {
				s.host.Log("error", fmt.Sprintf("firmware job %d: mark interrupted on shutdown: %v", jobID, ierr))
			}
			return
		}
		result := firmware.CheckOneWithOptions(shutdown, device, s.host.FirmwareOptions(device, 10*time.Second))
		results = append(results, result)
		applyCheckResult(&device, result)
		if mode, autoErr := firmware.ReadAutoUpdate(shutdown, device.IP, device.Gen, s.host.FirmwareOptions(device, 5*time.Second)); autoErr == nil {
			device.FWAutoUpdate = mode
		}
		if methods, mErr := firmware.ListSupportedMethods(shutdown, device.IP, device.Gen, s.host.FirmwareOptions(device, 5*time.Second)); mErr == nil {
			device.SupportedMethods = methods
		}
		if uerr := s.store.UpsertDevice(device); uerr != nil {
			s.host.Log("error", fmt.Sprintf("firmware job %d: persist fw cache for %s: %v", jobID, device.MAC, uerr))
		}
		body, merr := json.Marshal(FirmwareJobResult{Results: results})
		if merr != nil {
			s.host.Log("error", fmt.Sprintf("firmware job %d: marshal progress body failed: %v", jobID, merr))
			continue
		}
		if perr := s.store.UpdateJobProgress(jobID, len(results), len(devices), string(body)); perr != nil {
			s.host.Log("error", fmt.Sprintf("firmware job %d: update progress failed: %v", jobID, perr))
		}
	}
	body, merr := json.Marshal(FirmwareJobResult{Results: results})
	if merr != nil {
		s.host.Log("error", fmt.Sprintf("firmware job %d: marshal final body failed: %v", jobID, merr))
	}
	if cerr := s.store.CompleteJob(jobID, "completed", string(body), "", len(results), len(devices)); cerr != nil {
		s.host.Log("error", fmt.Sprintf("firmware job %d: complete-success write failed: %v", jobID, cerr))
	}
}

// RunFirmwareCheckScheduler periodically triggers a firmware_check job at
// the cadence configured via AppSettings.FirmwareCheckInterval (seconds;
// 0 = disabled). Polls the setting every minute so live changes apply
// without a service restart. Skips ticks when a firmware_check is already
// running (idempotent under concurrent operator-initiated checks).
func (s *Service) RunFirmwareCheckScheduler() {
	const pollInterval = 60 * time.Second
	shutdown := s.host.ShutdownContext()
	var nextRun time.Time
	for {
		select {
		case <-shutdown.Done():
			return
		case <-time.After(pollInterval):
		}
		settings, err := s.store.GetSettings()
		if err != nil {
			continue
		}
		settings.Normalize()
		var emit bool
		nextRun, emit = FirmwareSchedulerDecision(time.Now(), settings.FirmwareCheckInterval, nextRun)
		if !emit {
			continue
		}
		if _, err := s.StartFirmwareCheck(); err != nil {
			s.host.Log("info", fmt.Sprintf("scheduled firmware check skipped: %v", err))
		} else {
			s.host.Log("info", "scheduled firmware check started")
		}
	}
}

// FirmwareStatus reports the latest firmware_check job's progress + the
// per-device results captured so far.
func (s *Service) FirmwareStatus() (FirmwareStatus, error) {
	job, err := s.store.GetLatestJob("firmware_check")
	if err != nil {
		return FirmwareStatus{Results: []firmware.Result{}}, nil
	}
	result, _ := ParseFirmwareResult(job.Result)
	return FirmwareStatus{
		Running: job.Status == "running",
		Done:    job.Done,
		Total:   job.Total,
		Results: result.Results,
	}, nil
}

// FirmwareUpdate triggers a one-shot firmware update for the given MACs
// and returns synchronously. Used by the per-device "firmware_update"
// action; the bulk Update page goes through StartFirmwareInstall instead.
// Reserves each MAC against the activeFirmware set on the host so it can't
// race with an in-flight Provision against the same target.
func (s *Service) FirmwareUpdate(ctx context.Context, macs []string, stage string) ([]firmware.UpdateResult, error) {
	if stage == "" {
		stage = "stable"
	}
	devices, err := s.store.ListDevices()
	if err != nil {
		return nil, err
	}
	index := map[string]models.Device{}
	for _, device := range devices {
		index[device.MAC] = device
	}
	requested := make([]string, 0, len(macs))
	for _, mac := range macs {
		if _, ok := index[mac]; ok {
			requested = append(requested, "mac:"+mac)
		}
	}
	allowed, skipped := s.host.ReserveFirmwareTargets(requested)
	defer s.host.ReleaseFirmwareTargets(allowed)

	allowedSet := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = true
	}

	results := make([]firmware.UpdateResult, 0, len(macs)+len(skipped))
	for _, key := range skipped {
		mac := strings.TrimPrefix(key, "mac:")
		if device, ok := index[mac]; ok {
			results = append(results, firmware.UpdateResult{
				IP:     device.IP,
				MAC:    mac,
				Status: "skipped",
				Detail: "device busy with provisioning",
			})
		}
	}
	for _, mac := range macs {
		if device, ok := index[mac]; ok {
			if !allowedSet["mac:"+mac] {
				continue
			}
			r := firmware.TriggerUpdateWithOptions(ctx, device.IP, device.Gen, stage, s.host.FirmwareOptions(device, 10*time.Second))
			r.MAC = mac
			results = append(results, r)
		}
	}
	return results, nil
}

// applyCheckResult folds one firmware.Result into the device row.
//
// The rule that matters: a FAILED check must not be written as if it had
// succeeded. Before this existed the job persisted `result.StableVer` /
// `BetaVer` / `CheckedAt` unconditionally, so a check against an unreachable
// device blanked the firmware cache and stamped a fresh "checked at" — the
// row then looked freshly verified while the job had in fact reached nothing.
// Reachability was never recorded at all, which is how a device with a stale
// IP kept reporting `online: true` and an empty `last_refresh_error` while
// every job against it failed with "no route to host" (and bulk actions,
// which gate on Online, happily targeted the dead address).
//
// Persist the cache only on success; on failure record what was actually
// learned — that the device refused, or that it did not answer. The
// reachability semantics mirror the refresh path: an answer that refuses
// keeps the device online, silence counts as a miss and takes it offline on
// the second one.
func applyCheckResult(device *models.Device, result firmware.Result) {
	// Opportunistic identity metadata is a real observation whenever it
	// arrives, including on a partially failed check.
	if result.CurrentVer != "" {
		device.FW = result.CurrentVer
	}
	if result.Batch != "" {
		device.Batch = result.Batch
	}
	if result.FWID != "" {
		device.FWID = result.FWID
	}

	switch result.Status {
	case "error":
		device.LastRefreshOK = false
		device.LastRefreshError = result.Note
		if result.Unreachable {
			device.ConsecutiveMisses++
			if device.ConsecutiveMisses >= 2 {
				device.Online = false
			}
		} else {
			// It answered — refused, but answered.
			device.ConsecutiveMisses = 0
			device.Online = true
		}
	case "na":
		// Not applicable (gen1). Says nothing about the device; leave the
		// row untouched beyond the identity fields above.
	default:
		device.FWAvailableStable = result.StableVer
		device.FWAvailableBeta = result.BetaVer
		device.FWCheckedAt = result.CheckedAt
		device.LastRefreshOK = true
		device.LastRefreshError = ""
		device.ConsecutiveMisses = 0
		device.Online = true
	}
}
