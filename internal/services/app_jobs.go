package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"shellyadmin/internal/core/firmware"
	"shellyadmin/internal/core/scanner"
	"shellyadmin/internal/models"
)

const staleScanGrace = 15 * time.Second
const staleRefreshGrace = 2 * time.Minute

type ScanStatus struct {
	Running bool             `json:"running"`
	Found   int              `json:"found"`
	Total   int              `json:"total"`
	Done    int              `json:"done"`
	Pending []map[string]any `json:"pending"`
}

type FirmwareStatus struct {
	Running bool              `json:"running"`
	Done    int               `json:"done"`
	Total   int               `json:"total"`
	Results []firmware.Result `json:"results"`
}

type ScanJobPayload struct {
	ExistingMACs []string `json:"existing_macs"`
}

type ScanJobResult struct {
	Pending []models.Device `json:"pending"`
}

type FirmwareJobResult struct {
	Results []firmware.Result `json:"results"`
}

func refreshProbeTimeout(settings models.AppSettings) time.Duration {
	settings.Normalize()
	return time.Duration(settings.RefreshTimeout * float64(time.Second))
}

func scanJobStale(job models.Job, now time.Time) (bool, error) {
	updatedAt, err := time.Parse(time.RFC3339, job.UpdatedAt)
	if err != nil {
		return false, err
	}
	return now.Sub(updatedAt) > staleScanGrace, nil
}

func refreshJobStale(job models.Job, now time.Time) (bool, error) {
	updatedAt, err := time.Parse(time.RFC3339, job.UpdatedAt)
	if err != nil {
		return false, err
	}
	return now.Sub(updatedAt) > staleRefreshGrace, nil
}

// --- Refresh ---

func (s *AppService) RefreshDevices(ctx context.Context) ([]models.Device, error) {
	if latest, err := s.db.GetLatestJob("refresh"); err == nil && latest.Status == "running" {
		stale, staleErr := refreshJobStale(latest, time.Now())
		if staleErr == nil && stale {
			if ierr := s.db.InterruptJob(latest.ID, "refresh stalled"); ierr != nil {
				s.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: mark stalled failed: %v", latest.ID, ierr))
			}
		} else {
			return nil, errors.New("refresh already running")
		}
	}
	jobID, err := s.db.CreateJob("refresh", "auto", "{}", 0)
	if err != nil {
		return nil, err
	}
	jobCtx, cancel := s.linkedContext(ctx)
	done := make(chan error, 1)
	s.bgJobs.Add(1)
	go func() {
		defer cancel()
		defer s.bgJobs.Done()
		s.runRefreshJob(jobCtx, jobID, done)
	}()
	if err := <-done; err != nil {
		return nil, err
	}
	return s.GetDevices()
}

func (s *AppService) runRefreshJob(ctx context.Context, jobID int64, done chan<- error) {
	devices, err := s.db.ListDevices()
	if err != nil {
		if cerr := s.db.CompleteJob(jobID, "failed", "", err.Error(), 0, 0); cerr != nil {
			s.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: complete-failed write failed: %v", jobID, cerr))
		}
		done <- err
		return
	}
	settings, err := s.db.GetSettings()
	if err != nil {
		if cerr := s.db.CompleteJob(jobID, "failed", "", err.Error(), 0, 0); cerr != nil {
			s.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: complete-failed write failed: %v", jobID, cerr))
		}
		done <- err
		return
	}
	timeout := refreshProbeTimeout(settings)
	limit := BoundedConcurrency(settings.ScanConcurrency)
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
	if perr := s.db.UpdateJobProgress(jobID, 0, len(devices), ""); perr != nil {
		s.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: initial progress update failed: %v", jobID, perr))
	}

	for i := 0; i < limit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for device := range work {
				select {
				case <-ctx.Done():
					if ierr := s.db.IncrementJobDone(jobID); ierr != nil {
						s.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: increment done failed: %v", jobID, ierr))
					}
					return
				default:
				}
				attemptedAt := time.Now().UTC().Format(time.RFC3339)
				updated := device
				probeOpts := s.scannerProbeOptions(device, timeout)
				if found := scanner.ProbeDeviceWithOptions(ctx, device.IP, probeOpts, s.Log); found != nil && !found.AuthRequired {
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
					// these); refreshFirmwareCache below overwrites on success.
					found.FWAvailableStable = device.FWAvailableStable
					found.FWAvailableBeta = device.FWAvailableBeta
					found.FWCheckedAt = device.FWCheckedAt
					found.FWAutoUpdate = device.FWAutoUpdate
					s.refreshFirmwareCache(ctx, found)
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
				if ierr := s.db.IncrementJobDone(jobID); ierr != nil {
					s.Log("error", fmt.Sprintf("refresh job %d: increment done failed: %v", jobID, ierr))
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
		if ierr := s.db.InterruptJob(jobID, "service shutdown"); ierr != nil {
			s.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: mark interrupted on shutdown: %v", jobID, ierr))
		}
		done <- ctx.Err()
		return
	}
	if err := s.db.UpsertDevices(refreshed); err != nil {
		if cerr := s.db.CompleteJob(jobID, "failed", "", err.Error(), len(devices), len(devices)); cerr != nil {
			s.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: complete-failed write failed: %v", jobID, cerr))
		}
		done <- err
		return
	}
	body, err := json.Marshal(map[string]any{"refreshed": len(refreshed)})
	if err != nil {
		s.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: marshal result body failed: %v", jobID, err))
	}
	if cerr := s.db.CompleteJob(jobID, "completed", string(body), "", len(devices), len(devices)); cerr != nil {
		s.LogCtx(ctx, "error", fmt.Sprintf("refresh job %d: complete-success write failed: %v", jobID, cerr))
	}
	done <- nil
}

// --- Scan ---

func (s *AppService) StartScan() error {
	settings, err := s.db.GetSettings()
	if err != nil {
		return err
	}
	if err := ValidateSettings(settings); err != nil {
		return err
	}
	if latest, err := s.db.GetLatestJob("scan"); err == nil && latest.Status == "running" {
		stale, staleErr := scanJobStale(latest, time.Now())
		if staleErr == nil && stale {
			if ierr := s.db.InterruptJob(latest.ID, "scan stalled"); ierr != nil {
				s.Log("error", fmt.Sprintf("scan job %d: mark stalled failed: %v", latest.ID, ierr))
			}
		} else {
			return errors.New("scan already running")
		}
	}
	devices, err := s.db.ListDevices()
	if err != nil {
		return err
	}
	existingMACs := make([]string, 0, len(devices))
	total := 0
	for _, device := range devices {
		existingMACs = append(existingMACs, device.MAC)
	}
	for _, subnet := range settings.Subnets {
		ips, err := scanner.ExpandCIDR(subnet)
		if err != nil {
			return err
		}
		total += len(ips)
	}
	if settings.EnableMDNS {
		total++
	}
	if total > maxScanTargets {
		return fmt.Errorf("scan target count %d exceeds limit %d", total, maxScanTargets)
	}
	payload, _ := json.Marshal(ScanJobPayload{ExistingMACs: existingMACs})
	jobID, err := s.db.CreateJob("scan", "auto", string(payload), total)
	if err != nil {
		return err
	}
	s.bgJobs.Add(1)
	go func() {
		defer s.bgJobs.Done()
		s.runScanJob(jobID, settings)
	}()
	return nil
}

func (s *AppService) runScanJob(jobID int64, settings models.AppSettings) {
	settings.Normalize()
	timeout := time.Duration(settings.ScanTimeout * float64(time.Second))
	results := scanner.ScanSubnets(s.ctx, settings.Subnets, BoundedConcurrency(settings.ScanConcurrency), timeout, s.Log, func() {
		if ierr := s.db.IncrementJobDone(jobID); ierr != nil {
			s.Log("error", fmt.Sprintf("scan job %d: increment done failed: %v", jobID, ierr))
		}
	})
	if settings.EnableMDNS && s.ctx.Err() == nil {
		mdnsResults := scanner.ScanMDNS(s.ctx, timeout, s.Log)
		results = scanner.MergeDevices(results, mdnsResults)
		if ierr := s.db.IncrementJobDone(jobID); ierr != nil {
			s.Log("error", fmt.Sprintf("scan job %d: increment done (mdns) failed: %v", jobID, ierr))
		}
	}
	if s.ctx.Err() != nil {
		if ierr := s.db.InterruptJob(jobID, "service shutdown"); ierr != nil {
			s.Log("error", fmt.Sprintf("scan job %d: mark interrupted on shutdown: %v", jobID, ierr))
		}
		return
	}
	body, err := json.Marshal(ScanJobResult{Pending: results})
	if err != nil {
		s.Log("error", fmt.Sprintf("scan job %d: marshal result body failed: %v", jobID, err))
	}
	job, err := s.db.GetJob(jobID)
	if err != nil {
		s.Log("error", fmt.Sprintf("scan job %d: lookup for completion failed: %v", jobID, err))
		return
	}
	if cerr := s.db.CompleteJob(jobID, "completed", string(body), "", job.Total, job.Total); cerr != nil {
		s.Log("error", fmt.Sprintf("scan job %d: complete-success write failed: %v", jobID, cerr))
	}
}

func (s *AppService) ScanStatus() (ScanStatus, error) {
	job, err := s.db.GetLatestJob("scan")
	if err != nil {
		return ScanStatus{Pending: []map[string]any{}}, nil
	}
	if job.Status == "running" {
		if stale, staleErr := scanJobStale(job, time.Now()); staleErr == nil && stale {
			if ierr := s.db.InterruptJob(job.ID, "scan stalled"); ierr != nil {
				s.Log("error", fmt.Sprintf("scan job %d: mark stalled failed: %v", job.ID, ierr))
			}
			job, err = s.db.GetJob(job.ID)
			if err != nil {
				return ScanStatus{Pending: []map[string]any{}}, nil
			}
		}
	}
	payload, perr := ParseScanPayload(job.Payload)
	if perr != nil {
		s.Log("warn", fmt.Sprintf("scan status: parse payload for job %d: %v", job.ID, perr))
	}
	result, rerr := ParseScanResult(job.Result)
	if rerr != nil {
		s.Log("warn", fmt.Sprintf("scan status: parse result for job %d: %v", job.ID, rerr))
	}
	existing := map[string]bool{}
	for _, mac := range payload.ExistingMACs {
		existing[mac] = true
	}
	pending := make([]map[string]any, 0, len(result.Pending))
	for _, device := range result.Pending {
		body, merr := json.Marshal(device)
		if merr != nil {
			s.Log("warn", fmt.Sprintf("scan status: marshal pending device %s: %v", device.MAC, merr))
			continue
		}
		var raw map[string]any
		if uerr := json.Unmarshal(body, &raw); uerr != nil {
			s.Log("warn", fmt.Sprintf("scan status: unmarshal pending device %s: %v", device.MAC, uerr))
			continue
		}
		raw["is_new"] = !existing[device.MAC]
		pending = append(pending, raw)
	}
	return ScanStatus{
		Running: job.Status == "running",
		Found:   len(result.Pending),
		Total:   job.Total,
		Done:    job.Done,
		Pending: pending,
	}, nil
}

func (s *AppService) ConfirmScan(macs []string) (int, error) {
	job, err := s.db.GetLatestJob("scan")
	if err != nil {
		return 0, errors.New("no scan job available")
	}
	result, _ := ParseScanResult(job.Result)
	selected := make([]models.Device, 0, len(result.Pending))
	remaining := make([]models.Device, 0, len(result.Pending))
	if len(macs) == 0 {
		selected = result.Pending
	} else {
		wanted := map[string]bool{}
		for _, mac := range macs {
			wanted[mac] = true
		}
		for _, device := range result.Pending {
			if wanted[device.MAC] {
				selected = append(selected, device)
			} else {
				remaining = append(remaining, device)
			}
		}
	}
	if err := s.db.UpsertDevices(selected); err != nil {
		return 0, err
	}
	if len(macs) == 0 {
		remaining = []models.Device{}
	}
	if body, err := json.Marshal(ScanJobResult{Pending: remaining}); err == nil {
		if perr := s.db.UpdateJobProgress(job.ID, job.Done, job.Total, string(body)); perr != nil {
			s.Log("error", fmt.Sprintf("scan job %d: update progress after accept failed: %v", job.ID, perr))
		}
	}
	return len(selected), nil
}

// --- Firmware check ---

const defaultFirmwareInstallTimeout = 5 * time.Minute
const firmwareInstallPollInterval = 5 * time.Second
const firmwareInstallConcurrency = 5

func (s *AppService) StartFirmwareCheck() (int, error) {
	if latest, err := s.db.GetLatestJob("firmware_check"); err == nil && latest.Status == "running" {
		return latest.Total, errors.New("firmware check already running")
	}
	devices, err := s.db.ListDevices()
	if err != nil {
		return 0, err
	}
	jobID, err := s.db.CreateJob("firmware_check", "auto", "{}", len(devices))
	if err != nil {
		return 0, err
	}
	s.bgJobs.Add(1)
	go func() {
		defer s.bgJobs.Done()
		s.runFirmwareJob(jobID, devices)
	}()
	return len(devices), nil
}

func (s *AppService) runFirmwareJob(jobID int64, devices []models.Device) {
	results := make([]firmware.Result, 0, len(devices))
	for _, device := range devices {
		if s.ctx.Err() != nil {
			if ierr := s.db.InterruptJob(jobID, "service shutdown"); ierr != nil {
				s.Log("error", fmt.Sprintf("firmware job %d: mark interrupted on shutdown: %v", jobID, ierr))
			}
			return
		}
		result := firmware.CheckOneWithOptions(s.ctx, device, s.firmwareOptions(device, 10*time.Second))
		results = append(results, result)
		// Persist the per-channel cache so the channel selector on the Update
		// page is purely a display filter and other pages (Devices, etc.) can
		// surface availability without a fresh check. Also write back the
		// running version (from GetDeviceInfo) so out-of-band upgrades stop
		// leaving Device.FW stale.
		if result.CurrentVer != "" {
			device.FW = result.CurrentVer
		}
		device.FWAvailableStable = result.StableVer
		device.FWAvailableBeta = result.BetaVer
		device.FWCheckedAt = result.CheckedAt
		if mode, autoErr := firmware.ReadAutoUpdate(s.ctx, device.IP, device.Gen, s.firmwareOptions(device, 5*time.Second)); autoErr == nil {
			device.FWAutoUpdate = mode
		}
		if uerr := s.db.UpsertDevice(device); uerr != nil {
			s.Log("error", fmt.Sprintf("firmware job %d: persist fw cache for %s: %v", jobID, device.MAC, uerr))
		}
		body, merr := json.Marshal(FirmwareJobResult{Results: results})
		if merr != nil {
			s.Log("error", fmt.Sprintf("firmware job %d: marshal progress body failed: %v", jobID, merr))
			continue
		}
		if perr := s.db.UpdateJobProgress(jobID, len(results), len(devices), string(body)); perr != nil {
			s.Log("error", fmt.Sprintf("firmware job %d: update progress failed: %v", jobID, perr))
		}
	}
	body, merr := json.Marshal(FirmwareJobResult{Results: results})
	if merr != nil {
		s.Log("error", fmt.Sprintf("firmware job %d: marshal final body failed: %v", jobID, merr))
	}
	if cerr := s.db.CompleteJob(jobID, "completed", string(body), "", len(results), len(devices)); cerr != nil {
		s.Log("error", fmt.Sprintf("firmware job %d: complete-success write failed: %v", jobID, cerr))
	}
}

// runFirmwareCheckScheduler periodically triggers a firmware_check job at
// the cadence configured via AppSettings.FirmwareCheckInterval (seconds; 0
// = disabled). Polls the setting every minute so live changes apply without
// a service restart. Skips ticks when a firmware_check is already running
// (idempotent under concurrent operator-initiated checks).
func (s *AppService) runFirmwareCheckScheduler() {
	const pollInterval = 60 * time.Second
	var nextRun time.Time
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(pollInterval):
		}
		settings, err := s.db.GetSettings()
		if err != nil {
			continue
		}
		settings.Normalize()
		var emit bool
		nextRun, emit = firmwareSchedulerDecision(time.Now(), settings.FirmwareCheckInterval, nextRun)
		if !emit {
			continue
		}
		if _, err := s.StartFirmwareCheck(); err != nil {
			s.Log("info", fmt.Sprintf("scheduled firmware check skipped: %v", err))
		} else {
			s.Log("info", "scheduled firmware check started")
		}
	}
}

func (s *AppService) FirmwareStatus() (FirmwareStatus, error) {
	job, err := s.db.GetLatestJob("firmware_check")
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

// FirmwareUpdate triggers a one-shot firmware update for the given MACs and
// returns synchronously. Used by the per-device "firmware_update" action; the
// bulk Update page goes through StartFirmwareInstall instead.
func (s *AppService) FirmwareUpdate(ctx context.Context, macs []string, stage string) ([]firmware.UpdateResult, error) {
	if stage == "" {
		stage = "stable"
	}
	devices, err := s.db.ListDevices()
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
	allowed, skipped := s.reserveFirmwareTargets(requested)
	defer s.releaseFirmwareTargets(allowed)

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
			r := firmware.TriggerUpdateWithOptions(ctx, device.IP, device.Gen, stage, s.firmwareOptions(device, 10*time.Second))
			r.MAC = mac
			results = append(results, r)
		}
	}
	return results, nil
}

// --- Firmware install (bulk) ---

// FirmwareInstallResult is the per-device row tracked inside a firmware_install
// job. Status flows triggered → updating → current/error/unknown.
type FirmwareInstallResult struct {
	IP      string `json:"ip"`
	MAC     string `json:"mac"`
	Stage   string `json:"stage"`
	FromVer string `json:"from_ver"`
	ToVer   string `json:"to_ver"`
	Status  string `json:"status"`
	Detail  string `json:"detail"`
}

type FirmwareInstallJobPayload struct {
	MACs  []string `json:"macs"`
	Stage string   `json:"stage"`
}

type FirmwareInstallJobResult struct {
	Results []FirmwareInstallResult `json:"results"`
}

type FirmwareInstallStatus struct {
	Running bool                    `json:"running"`
	Done    int                     `json:"done"`
	Total   int                     `json:"total"`
	Results []FirmwareInstallResult `json:"results"`
}

// StartFirmwareInstall is the entry point used by the bulk Update page. It
// reserves each MAC, spawns a single background job that runs Shelly.Update
// with bounded concurrency, then polls each device's Shelly.GetDeviceInfo
// every firmwareInstallPollInterval until the reported version changes (or
// matches the per-channel target captured by the latest firmware_check),
// timing out at firmwareInstallTimeout.
func (s *AppService) StartFirmwareInstall(macs []string, stage string) (int64, int, error) {
	if stage == "" {
		stage = "stable"
	}
	if stage != "stable" && stage != "beta" {
		return 0, 0, fmt.Errorf("invalid stage %q", stage)
	}
	if latest, err := s.db.GetLatestJob("firmware_install"); err == nil && latest.Status == "running" {
		return latest.ID, latest.Total, errors.New("firmware install already running")
	}
	devices, err := s.db.ListDevices()
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
	jobID, err := s.db.CreateJob("firmware_install", "manual", string(payload), len(targetMACs))
	if err != nil {
		return 0, 0, err
	}
	timeout := defaultFirmwareInstallTimeout
	if settings, err := s.db.GetSettings(); err == nil {
		timeout = firmwareInstallTimeoutFromSettings(settings)
	}
	s.bgJobs.Add(1)
	go func() {
		defer s.bgJobs.Done()
		s.runFirmwareInstallJob(jobID, targetMACs, stage, index, timeout)
	}()
	return jobID, len(targetMACs), nil
}

func (s *AppService) runFirmwareInstallJob(jobID int64, macs []string, stage string, index map[string]models.Device, timeout time.Duration) {
	requested := make([]string, 0, len(macs))
	for _, mac := range macs {
		requested = append(requested, "mac:"+mac)
	}
	allowed, _ := s.reserveFirmwareTargets(requested)
	defer s.releaseFirmwareTargets(allowed)

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
			ToVer:   targetVersion(device, stage),
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
			if isInstallTerminal(r.Status) {
				done++
			}
		}
		resMu.Unlock()
		body, merr := json.Marshal(FirmwareInstallJobResult{Results: snapshot})
		if merr != nil {
			s.Log("error", fmt.Sprintf("firmware install job %d: marshal progress: %v", jobID, merr))
			return
		}
		if perr := s.db.UpdateJobProgress(jobID, done, len(snapshot), string(body)); perr != nil {
			s.Log("error", fmt.Sprintf("firmware install job %d: update progress: %v", jobID, perr))
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

	sem := make(chan struct{}, firmwareInstallConcurrency)
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
			s.installOne(jobID, i, mac, stage, index[mac], timeout, setResult, persistProgress)
		}()
	}
	wg.Wait()

	resMu.Lock()
	final := make([]FirmwareInstallResult, len(results))
	copy(final, results)
	resMu.Unlock()
	body, _ := json.Marshal(FirmwareInstallJobResult{Results: final})
	if cerr := s.db.CompleteJob(jobID, "completed", string(body), "", len(final), len(final)); cerr != nil {
		s.Log("error", fmt.Sprintf("firmware install job %d: complete write failed: %v", jobID, cerr))
	}
}

func (s *AppService) installOne(jobID int64, idx int, mac, stage string, device models.Device, timeout time.Duration, setResult func(int, func(*FirmwareInstallResult)), persistProgress func()) {
	if s.ctx.Err() != nil {
		setResult(idx, func(r *FirmwareInstallResult) {
			r.Status = "unknown"
			r.Detail = "service shutting down"
		})
		persistProgress()
		return
	}

	triggerCtx, triggerCancel := context.WithTimeout(s.ctx, 15*time.Second)
	trigger := firmware.TriggerUpdateWithOptions(triggerCtx, device.IP, device.Gen, stage, s.firmwareOptions(device, 10*time.Second))
	triggerCancel()
	if trigger.Status != "triggered" {
		setResult(idx, func(r *FirmwareInstallResult) {
			r.Status = "error"
			r.Detail = trigger.Detail
		})
		persistProgress()
		s.Log("warn", fmt.Sprintf("firmware install job %d: trigger %s failed: %s", jobID, mac, trigger.Detail))
		return
	}

	setResult(idx, func(r *FirmwareInstallResult) {
		r.Status = "updating"
		r.Detail = trigger.Detail
	})
	persistProgress()

	deadline := time.Now().Add(timeout)
	initialVer := device.FW
	expected := targetVersion(device, stage)
	for {
		if s.ctx.Err() != nil {
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
					r.Detail = fmt.Sprintf("device still on %s after %s (expected %s)", initialVer, formatTimeout(timeout), expected)
				} else {
					r.Detail = fmt.Sprintf("no version change detected after %s", formatTimeout(timeout))
				}
			})
			persistProgress()
			return
		}
		select {
		case <-s.ctx.Done():
			continue
		case <-time.After(firmwareInstallPollInterval):
		}
		probeCtx, probeCancel := context.WithTimeout(s.ctx, 8*time.Second)
		ver, err := firmware.GetDeviceFirmware(probeCtx, device.IP, device.Gen, s.firmwareOptions(device, 8*time.Second))
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
		fresh, err := s.db.ListDevices()
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
					_ = s.db.UpsertDevice(d)
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

func (s *AppService) FirmwareInstallStatus() (FirmwareInstallStatus, error) {
	job, err := s.db.GetLatestJob("firmware_install")
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

// firmwareInstallTimeoutFromSettings is the canonical conversion from the
// AppSettings field to a time.Duration. Pulled into a top-level helper so it
// can be unit-tested without spinning up an install job, and so any caller
// that needs the same value (test harness, future debug endpoint) doesn't
// re-implement the float-seconds-to-Duration math.
func firmwareInstallTimeoutFromSettings(s models.AppSettings) time.Duration {
	if s.FirmwareInstallTimeout > 0 {
		return time.Duration(s.FirmwareInstallTimeout * float64(time.Second))
	}
	return defaultFirmwareInstallTimeout
}

// firmwareSchedulerDecision is the per-tick logic of runFirmwareCheckScheduler.
// `now` is wall-clock time; `intervalSec` is the configured cadence (0 means
// disabled); `nextRun` is the previously-scheduled fire time (zero value =
// "anchor on first non-zero interval seen"). Returns the new nextRun anchor
// and whether the caller should fire StartFirmwareCheck right now.
func firmwareSchedulerDecision(now time.Time, intervalSec int, nextRun time.Time) (time.Time, bool) {
	if intervalSec <= 0 {
		return time.Time{}, false
	}
	if nextRun.IsZero() {
		return now.Add(time.Duration(intervalSec) * time.Second), false
	}
	if now.Before(nextRun) {
		return nextRun, false
	}
	return now.Add(time.Duration(intervalSec) * time.Second), true
}

// formatTimeout renders an install timeout as a short human phrase
// ("5 min", "90 sec") for the install_job's per-device detail line.
func formatTimeout(d time.Duration) string {
	if d >= time.Minute && d%time.Minute == 0 {
		return fmt.Sprintf("%d min", int(d/time.Minute))
	}
	if d >= time.Minute {
		return fmt.Sprintf("%.1f min", d.Minutes())
	}
	return fmt.Sprintf("%d sec", int(d.Seconds()))
}

func targetVersion(d models.Device, stage string) string {
	if stage == "beta" {
		return d.FWAvailableBeta
	}
	return d.FWAvailableStable
}

func isInstallTerminal(status string) bool {
	switch status {
	case "current", "error", "unknown", "skipped":
		return true
	}
	return false
}

// --- Recovery ---

func (s *AppService) RecoverInterruptedJobs() error {
	jobs, err := s.db.ListInterruptedRestartableJobs()
	if err != nil {
		return err
	}
	for _, job := range jobs {
		switch job.Type {
		case "scan":
			settings, err := s.db.GetSettings()
			if err != nil {
				continue
			}
			payload := job.Payload
			total := job.Total
			newJobID, err := s.db.CreateJob("scan", "auto", payload, total)
			if err != nil {
				continue
			}
			s.bgJobs.Add(1)
			go func(id int64, cfg models.AppSettings) {
				defer s.bgJobs.Done()
				s.runScanJob(id, cfg)
			}(newJobID, settings)
			s.Log("INFO", fmt.Sprintf("auto-restarted interrupted job scan:%d as job:%d", job.ID, newJobID))
		case "refresh":
			// Refresh jobs are lightweight read-only probes. Rather than
			// auto-restarting them on startup (which would briefly block the
			// user's manual refresh), simply leave them as interrupted and let
			// the user trigger a fresh refresh when ready.
		case "firmware_check":
			devices, err := s.db.ListDevices()
			if err != nil {
				continue
			}
			newJobID, err := s.db.CreateJob("firmware_check", "auto", "{}", len(devices))
			if err != nil {
				continue
			}
			s.bgJobs.Add(1)
			go func(id int64, devs []models.Device) {
				defer s.bgJobs.Done()
				s.runFirmwareJob(id, devs)
			}(newJobID, devices)
			s.Log("INFO", fmt.Sprintf("auto-restarted interrupted job firmware_check:%d as job:%d", job.ID, newJobID))
		}
	}
	return nil
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

// --- Job payload/result parsers ---

func ParseScanPayload(raw string) (ScanJobPayload, error) {
	if raw == "" {
		return ScanJobPayload{}, nil
	}
	var payload ScanJobPayload
	err := json.Unmarshal([]byte(raw), &payload)
	return payload, err
}

func ParseScanResult(raw string) (ScanJobResult, error) {
	if raw == "" {
		return ScanJobResult{Pending: []models.Device{}}, nil
	}
	var result ScanJobResult
	err := json.Unmarshal([]byte(raw), &result)
	return result, err
}

func ParseFirmwareResult(raw string) (FirmwareJobResult, error) {
	if raw == "" {
		return FirmwareJobResult{Results: []firmware.Result{}}, nil
	}
	var result FirmwareJobResult
	err := json.Unmarshal([]byte(raw), &result)
	return result, err
}
