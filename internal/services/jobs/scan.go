// MOVED FROM internal/services/app_jobs.go — v0.3.0 services-layer split
// (M7, docs/plans/phase-4b-refactor-block.md Block 4b.1.4). StartScan +
// runScanJob + ScanStatus + ConfirmScan lift onto *Service with the
// AppService receiver references rewritten as s.store.* (DB) and s.host.*
// (lifecycle, logger, validator).

package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"shellyadmin/internal/core/scanner"
	"shellyadmin/internal/models"
)

// MaxScanTargets caps the total subnet+mDNS work an operator can configure
// in one scan so a /16 typo doesn't try to probe 65k IPs.
const MaxScanTargets = 65534

// StartScan reserves a scan job and spawns the worker goroutine.
// ValidateSettings gates malformed configs (subnet count, timeouts) so a
// busted settings row doesn't burn a job row before failing.
func (s *Service) StartScan() error {
	settings, err := s.store.GetSettings()
	if err != nil {
		return err
	}
	// The raw DB row carries a secretbox-encrypted MCPToken, not plaintext.
	// ValidateSettings checks the URL-safe alphabet, so pass a copy with the
	// MCP fields cleared — the token was already validated at save time and
	// is irrelevant to scan-parameter validation.
	sv := settings
	sv.MCPToken = ""
	sv.MCPEnabled = false
	if err := s.host.ValidateSettings(sv); err != nil {
		return err
	}
	if latest, err := s.store.GetLatestJob("scan"); err == nil && latest.Status == "running" {
		stale, staleErr := ScanJobStale(latest, time.Now())
		if staleErr == nil && stale {
			if ierr := s.store.InterruptJob(latest.ID, "scan stalled"); ierr != nil {
				s.host.Log("error", fmt.Sprintf("scan job %d: mark stalled failed: %v", latest.ID, ierr))
			}
		} else {
			return errors.New("scan already running")
		}
	}
	devices, err := s.store.ListDevices()
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
	if total > MaxScanTargets {
		return fmt.Errorf("scan target count %d exceeds limit %d", total, MaxScanTargets)
	}
	payload, _ := json.Marshal(ScanJobPayload{ExistingMACs: existingMACs})
	jobID, err := s.store.CreateJob("scan", "auto", string(payload), total)
	if err != nil {
		return err
	}
	bg := s.host.BackgroundJobs()
	bg.Add(1)
	go func() {
		defer bg.Done()
		s.runScanJob(jobID, settings)
	}()
	return nil
}

// RunScanJob is the exported worker entry point. Used by StartScan and
// the recovery path (services.RecoverInterruptedJobs auto-restarts
// interrupted scans by calling this with the persisted payload).
func (s *Service) RunScanJob(jobID int64, settings models.AppSettings) {
	s.runScanJob(jobID, settings)
}

func (s *Service) runScanJob(jobID int64, settings models.AppSettings) {
	settings.Normalize()
	timeout := time.Duration(settings.ScanTimeout * float64(time.Second))
	shutdown := s.host.ShutdownContext()
	results := scanner.ScanSubnets(shutdown, settings.Subnets, boundedConcurrency(settings.ScanConcurrency), timeout, s.host.Log, func() {
		if ierr := s.store.IncrementJobDone(jobID); ierr != nil {
			s.host.Log("error", fmt.Sprintf("scan job %d: increment done failed: %v", jobID, ierr))
		}
	})
	if settings.EnableMDNS && shutdown.Err() == nil {
		mdnsResults := scanner.ScanMDNS(shutdown, timeout, s.host.Log)
		results = scanner.MergeDevices(results, mdnsResults)
		if ierr := s.store.IncrementJobDone(jobID); ierr != nil {
			s.host.Log("error", fmt.Sprintf("scan job %d: increment done (mdns) failed: %v", jobID, ierr))
		}
	}
	if shutdown.Err() != nil {
		if ierr := s.store.InterruptJob(jobID, "service shutdown"); ierr != nil {
			s.host.Log("error", fmt.Sprintf("scan job %d: mark interrupted on shutdown: %v", jobID, ierr))
		}
		return
	}
	body, err := json.Marshal(ScanJobResult{Pending: results})
	if err != nil {
		s.host.Log("error", fmt.Sprintf("scan job %d: marshal result body failed: %v", jobID, err))
	}
	job, err := s.store.GetJob(jobID)
	if err != nil {
		s.host.Log("error", fmt.Sprintf("scan job %d: lookup for completion failed: %v", jobID, err))
		return
	}
	if cerr := s.store.CompleteJob(jobID, "completed", string(body), "", job.Total, job.Total); cerr != nil {
		s.host.Log("error", fmt.Sprintf("scan job %d: complete-success write failed: %v", jobID, cerr))
	}
}

// ScanStatus reports the latest scan job's state plus its pending devices
// annotated with is_new (true for MACs not in the existing inventory at
// scan-spawn time).
func (s *Service) ScanStatus() (ScanStatus, error) {
	job, err := s.store.GetLatestJob("scan")
	if err != nil {
		return ScanStatus{Pending: []map[string]any{}}, nil
	}
	if job.Status == "running" {
		if stale, staleErr := ScanJobStale(job, time.Now()); staleErr == nil && stale {
			if ierr := s.store.InterruptJob(job.ID, "scan stalled"); ierr != nil {
				s.host.Log("error", fmt.Sprintf("scan job %d: mark stalled failed: %v", job.ID, ierr))
			}
			job, err = s.store.GetJob(job.ID)
			if err != nil {
				return ScanStatus{Pending: []map[string]any{}}, nil
			}
		}
	}
	payload, perr := ParseScanPayload(job.Payload)
	if perr != nil {
		s.host.Log("warn", fmt.Sprintf("scan status: parse payload for job %d: %v", job.ID, perr))
	}
	result, rerr := ParseScanResult(job.Result)
	if rerr != nil {
		s.host.Log("warn", fmt.Sprintf("scan status: parse result for job %d: %v", job.ID, rerr))
	}
	existing := map[string]bool{}
	for _, mac := range payload.ExistingMACs {
		existing[mac] = true
	}
	pending := make([]map[string]any, 0, len(result.Pending))
	for _, device := range result.Pending {
		body, merr := json.Marshal(device)
		if merr != nil {
			s.host.Log("warn", fmt.Sprintf("scan status: marshal pending device %s: %v", device.MAC, merr))
			continue
		}
		var raw map[string]any
		if uerr := json.Unmarshal(body, &raw); uerr != nil {
			s.host.Log("warn", fmt.Sprintf("scan status: unmarshal pending device %s: %v", device.MAC, uerr))
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

// ConfirmScan accepts a subset (or all) of the latest scan's pending
// devices into the persistent inventory. Empty macs accepts all and
// blanks the pending list; a populated macs accepts only the matching
// rows and keeps the rest pending.
func (s *Service) ConfirmScan(macs []string) (int, error) {
	job, err := s.store.GetLatestJob("scan")
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
	if err := s.store.UpsertDevices(selected); err != nil {
		return 0, err
	}
	if len(macs) == 0 {
		remaining = []models.Device{}
	}
	if body, err := json.Marshal(ScanJobResult{Pending: remaining}); err == nil {
		if perr := s.store.UpdateJobProgress(job.ID, job.Done, job.Total, string(body)); perr != nil {
			s.host.Log("error", fmt.Sprintf("scan job %d: update progress after accept failed: %v", job.ID, perr))
		}
	}
	return len(selected), nil
}
