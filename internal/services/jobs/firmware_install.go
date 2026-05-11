// MOVED FROM internal/services/app_jobs.go — v0.3.0 services-layer split
// (M7, docs/plans/phase-4b-refactor-block.md Block 4b.1.4). The bulk
// firmware-install flow (StartFirmwareInstall + runFirmwareInstallJob +
// installOne + FirmwareInstallStatus) lifts onto *Service with the
// AppService receiver references rewritten as s.store.* (DB) and s.host.*
// (lifecycle, reservation, RPC factories).

package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"shellyadmin/internal/core/firmware"
	"shellyadmin/internal/models"
)

// StartFirmwareInstall is the entry point used by the bulk Update page.
// It reserves each MAC, spawns a single background job that runs
// Shelly.Update with bounded concurrency, then polls each device's
// Shelly.GetDeviceInfo at the configured poll interval until the
// reported version changes (or matches the per-channel target captured
// by the latest firmware_check), timing out at firmwareInstallTimeout.
func (s *Service) StartFirmwareInstall(macs []string, stage string) (int64, int, error) {
	if stage == "" {
		stage = "stable"
	}
	if stage != "stable" && stage != "beta" {
		return 0, 0, fmt.Errorf("invalid stage %q", stage)
	}
	if latest, err := s.store.GetLatestJob("firmware_install"); err == nil && latest.Status == "running" {
		return latest.ID, latest.Total, errors.New("firmware install already running")
	}
	devices, err := s.store.ListDevices()
	if err != nil {
		return 0, 0, err
	}
	index := map[string]models.Device{}
	for _, device := range devices {
		index[device.MAC] = device
	}
	targetMACs := make([]string, 0, len(macs))
	for _, mac := range macs {
		if _, ok := index[mac]; ok {
			targetMACs = append(targetMACs, mac)
		}
	}
	if len(targetMACs) == 0 {
		return 0, 0, errors.New("no valid devices for install")
	}
	payload, err := json.Marshal(FirmwareInstallJobPayload{MACs: targetMACs, Stage: stage})
	if err != nil {
		return 0, 0, err
	}
	jobID, err := s.store.CreateJob("firmware_install", "manual", string(payload), len(targetMACs))
	if err != nil {
		return 0, 0, err
	}
	timeout := DefaultFirmwareInstallTimeout
	pollInterval := DefaultFirmwareInstallPollInterval
	if settings, err := s.store.GetSettings(); err == nil {
		timeout = FirmwareInstallTimeoutFromSettings(settings)
		pollInterval = FirmwareInstallPollIntervalFromSettings(settings)
	}
	bg := s.host.BackgroundJobs()
	bg.Add(1)
	go func() {
		defer bg.Done()
		s.runFirmwareInstallJob(jobID, targetMACs, stage, index, timeout, pollInterval)
	}()
	return jobID, len(targetMACs), nil
}

func (s *Service) runFirmwareInstallJob(jobID int64, macs []string, stage string, index map[string]models.Device, timeout time.Duration, pollInterval time.Duration) {
	requested := make([]string, 0, len(macs))
	for _, mac := range macs {
		requested = append(requested, "mac:"+mac)
	}
	allowed, _ := s.host.ReserveFirmwareTargets(requested)
	defer s.host.ReleaseFirmwareTargets(allowed)

	allowedSet := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = true
	}

	results := make([]FirmwareInstallResult, len(macs))
	for i, mac := range macs {
		device := index[mac]
		results[i] = FirmwareInstallResult{
			IP:      device.IP,
			MAC:     mac,
			Stage:   stage,
			FromVer: device.FW,
			ToVer:   TargetVersion(device, stage),
			Status:  "pending",
		}
	}

	var resMu sync.Mutex
	persistProgress := func() {
		resMu.Lock()
		snapshot := make([]FirmwareInstallResult, len(results))
		copy(snapshot, results)
		done := 0
		for _, r := range snapshot {
			if IsInstallTerminal(r.Status) {
				done++
			}
		}
		resMu.Unlock()
		body, merr := json.Marshal(FirmwareInstallJobResult{Results: snapshot})
		if merr != nil {
			s.host.Log("error", fmt.Sprintf("firmware install job %d: marshal progress: %v", jobID, merr))
			return
		}
		if perr := s.store.UpdateJobProgress(jobID, done, len(snapshot), string(body)); perr != nil {
			s.host.Log("error", fmt.Sprintf("firmware install job %d: update progress: %v", jobID, perr))
		}
	}
	setResult := func(i int, mut func(*FirmwareInstallResult)) {
		resMu.Lock()
		mut(&results[i])
		resMu.Unlock()
	}

	for i := range macs {
		if !allowedSet["mac:"+macs[i]] {
			setResult(i, func(r *FirmwareInstallResult) {
				r.Status = "skipped"
				r.Detail = "device busy with provisioning"
			})
		}
	}
	persistProgress()

	sem := make(chan struct{}, FirmwareInstallConcurrency)
	var wg sync.WaitGroup
	for i, mac := range macs {
		if !allowedSet["mac:"+mac] {
			continue
		}
		i, mac := i, mac
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			s.installOne(jobID, i, mac, stage, index[mac], timeout, pollInterval, setResult, persistProgress)
		}()
	}
	wg.Wait()

	resMu.Lock()
	final := make([]FirmwareInstallResult, len(results))
	copy(final, results)
	resMu.Unlock()
	body, _ := json.Marshal(FirmwareInstallJobResult{Results: final})
	if cerr := s.store.CompleteJob(jobID, "completed", string(body), "", len(final), len(final)); cerr != nil {
		s.host.Log("error", fmt.Sprintf("firmware install job %d: complete write failed: %v", jobID, cerr))
	}
}

func (s *Service) installOne(jobID int64, idx int, mac, stage string, device models.Device, timeout time.Duration, pollInterval time.Duration, setResult func(int, func(*FirmwareInstallResult)), persistProgress func()) {
	shutdown := s.host.ShutdownContext()
	if shutdown.Err() != nil {
		setResult(idx, func(r *FirmwareInstallResult) {
			r.Status = "unknown"
			r.Detail = "service shutting down"
		})
		persistProgress()
		return
	}

	triggerCtx, triggerCancel := context.WithTimeout(shutdown, 15*time.Second)
	trigger := firmware.TriggerUpdateWithOptions(triggerCtx, device.IP, device.Gen, stage, s.host.FirmwareOptions(device, 10*time.Second))
	triggerCancel()
	if trigger.Status != "triggered" {
		setResult(idx, func(r *FirmwareInstallResult) {
			r.Status = "error"
			r.Detail = trigger.Detail
		})
		persistProgress()
		s.host.Log("warn", fmt.Sprintf("firmware install job %d: trigger %s failed: %s", jobID, mac, trigger.Detail))
		return
	}

	setResult(idx, func(r *FirmwareInstallResult) {
		r.Status = "updating"
		r.Detail = trigger.Detail
	})
	persistProgress()

	deadline := time.Now().Add(timeout)
	initialVer := device.FW
	expected := TargetVersion(device, stage)
	for {
		if shutdown.Err() != nil {
			setResult(idx, func(r *FirmwareInstallResult) {
				r.Status = "unknown"
				r.Detail = "service shutting down"
			})
			persistProgress()
			return
		}
		if time.Now().After(deadline) {
			setResult(idx, func(r *FirmwareInstallResult) {
				r.Status = "unknown"
				if expected != "" {
					r.Detail = fmt.Sprintf("device still on %s after %s (expected %s)", initialVer, FormatTimeout(timeout), expected)
				} else {
					r.Detail = fmt.Sprintf("no version change detected after %s", FormatTimeout(timeout))
				}
			})
			persistProgress()
			return
		}
		select {
		case <-shutdown.Done():
			continue
		case <-time.After(pollInterval):
		}
		probeCtx, probeCancel := context.WithTimeout(shutdown, 8*time.Second)
		ver, err := firmware.GetDeviceFirmware(probeCtx, device.IP, device.Gen, s.host.FirmwareOptions(device, 8*time.Second))
		probeCancel()
		if err != nil || ver == "" {
			continue
		}
		// Success criteria: version matches the expected target (when known)
		// OR the version simply moved off the original.
		matched := (expected != "" && ver == expected) || (expected == "" && ver != initialVer)
		if !matched {
			continue
		}
		// Persist the new firmware on the device row and clear the channel's
		// available_ver since it's now installed.
		fresh, err := s.store.ListDevices()
		if err == nil {
			for _, d := range fresh {
				if d.MAC == mac {
					d.FW = ver
					if stage == "beta" && d.FWAvailableBeta == ver {
						d.FWAvailableBeta = ""
					}
					if stage == "stable" && d.FWAvailableStable == ver {
						d.FWAvailableStable = ""
					}
					d.FWCheckedAt = time.Now().UTC().Format(time.RFC3339)
					_ = s.store.UpsertDevice(d)
					break
				}
			}
		}
		setResult(idx, func(r *FirmwareInstallResult) {
			r.Status = "current"
			r.ToVer = ver
			r.Detail = "update completed"
		})
		persistProgress()
		return
	}
}

// FirmwareInstallStatus reports the latest firmware_install job's
// progress + per-device results.
func (s *Service) FirmwareInstallStatus() (FirmwareInstallStatus, error) {
	job, err := s.store.GetLatestJob("firmware_install")
	if err != nil {
		return FirmwareInstallStatus{Results: []FirmwareInstallResult{}}, nil
	}
	var result FirmwareInstallJobResult
	if job.Result != "" {
		_ = json.Unmarshal([]byte(job.Result), &result)
	}
	if result.Results == nil {
		result.Results = []FirmwareInstallResult{}
	}
	return FirmwareInstallStatus{
		Running: job.Status == "running",
		Done:    job.Done,
		Total:   job.Total,
		Results: result.Results,
	}, nil
}
