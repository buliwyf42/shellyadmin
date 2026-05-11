// MOVED FROM internal/services/app_jobs.go — v0.3.0 services-layer split
// (M7, docs/plans/phase-4b-refactor-block.md Block 4b.1.4). The startup
// recovery hook moves to jobs.Service since it spawns the same workers
// (RunScanJob / RunFirmwareJob) that already live here.

package jobs

import (
	"fmt"

	"shellyadmin/internal/models"
)

// RecoverInterruptedJobs is the boot-time hook that auto-restarts scan +
// firmware_check jobs left in the "interrupted" state by an unclean
// shutdown. Refresh jobs are intentionally NOT auto-restarted (they're
// lightweight read-only and would briefly block the user's manual
// refresh on startup — leave them as interrupted and let the user
// trigger a fresh one when ready).
func (s *Service) RecoverInterruptedJobs() error {
	pending, err := s.store.ListInterruptedRestartableJobs()
	if err != nil {
		return err
	}
	bg := s.host.BackgroundJobs()
	for _, job := range pending {
		switch job.Type {
		case "scan":
			settings, err := s.store.GetSettings()
			if err != nil {
				continue
			}
			payload := job.Payload
			total := job.Total
			newJobID, err := s.store.CreateJob("scan", "auto", payload, total)
			if err != nil {
				continue
			}
			bg.Add(1)
			go func(id int64, cfg models.AppSettings) {
				defer bg.Done()
				s.runScanJob(id, cfg)
			}(newJobID, settings)
			s.host.Log("INFO", fmt.Sprintf("auto-restarted interrupted job scan:%d as job:%d", job.ID, newJobID))
		case "refresh":
			// See doc comment above.
		case "firmware_check":
			devices, err := s.store.ListDevices()
			if err != nil {
				continue
			}
			newJobID, err := s.store.CreateJob("firmware_check", "auto", "{}", len(devices))
			if err != nil {
				continue
			}
			bg.Add(1)
			go func(id int64, devs []models.Device) {
				defer bg.Done()
				s.runFirmwareJob(id, devs)
			}(newJobID, devices)
			s.host.Log("INFO", fmt.Sprintf("auto-restarted interrupted job firmware_check:%d as job:%d", job.ID, newJobID))
		}
	}
	return nil
}
