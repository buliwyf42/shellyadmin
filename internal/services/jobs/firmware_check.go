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
		// Persist the per-channel cache so the channel selector on the
		// Update page is purely a display filter and other pages (Devices,
		// etc.) can surface availability without a fresh check. Also write
		// back the running version (from GetDeviceInfo) so out-of-band
		// upgrades stop leaving Device.FW stale.
		if result.CurrentVer != "" {
			device.FW = result.CurrentVer
		}
		if result.Batch != "" {
			device.Batch = result.Batch
		}
		if result.FWID != "" {
			device.FWID = result.FWID
		}
		device.FWAvailableStable = result.StableVer
		device.FWAvailableBeta = result.BetaVer
		device.FWCheckedAt = result.CheckedAt
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
