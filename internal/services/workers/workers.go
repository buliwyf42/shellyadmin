// Package workers owns the long-lived background goroutines AppService
// spawns at boot: session-row sweeper (S5), audit-log retention pruner
// (S1), auto-backup snapshotter (S12/S13), and the firmware-check
// scheduler-with-panic-recover wrapper (S9).
//
// MOVED FROM internal/services/app.go — v0.3.0 services-layer split (M7,
// docs/plans/phase-4b-refactor-block.md Block 4b.1). AppService keeps a
// *Service field and delegates StartBackgroundWorkers to it; the workers
// run for the lifetime of AppService and exit on its shutdown ctx.
package workers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"shellyadmin/internal/models"
)

// AuditRetentionTick is how often the retention pruner wakes up. Hourly
// keeps the worst-case tail short without burning observable CPU.
const AuditRetentionTick = time.Hour

// Store is the narrow persistence surface the workers need. *db.DB
// satisfies it structurally.
type Store interface {
	PruneExpiredSessions() (int64, error)
	PruneAuditLogOlderThan(cutoff time.Time) (int64, error)
	GetSettings() (models.AppSettings, error)
	SnapshotTo(path string) error
}

// Service hosts the worker loops.
type Service struct {
	store                Store
	ctx                  context.Context
	bgJobs               *sync.WaitGroup
	logf                 func(ctx context.Context, level, msg string)
	dataDir              string
	runFirmwareScheduler func()
}

// New constructs a Service. ctx is the host's shutdown context; bgJobs is
// the host's WaitGroup so Stop() drains the workers. logf is the
// audit-log writer; runFirmwareScheduler delegates to
// jobs.Service.RunFirmwareCheckScheduler. dataDir is where SnapshotTo
// writes auto-backup files.
func New(
	store Store,
	ctx context.Context,
	bgJobs *sync.WaitGroup,
	logf func(ctx context.Context, level, msg string),
	dataDir string,
	runFirmwareScheduler func(),
) *Service {
	if logf == nil {
		logf = func(context.Context, string, string) {}
	}
	if runFirmwareScheduler == nil {
		runFirmwareScheduler = func() {}
	}
	return &Service{
		store:                store,
		ctx:                  ctx,
		bgJobs:               bgJobs,
		logf:                 logf,
		dataDir:              dataDir,
		runFirmwareScheduler: runFirmwareScheduler,
	}
}

// Start spawns all worker goroutines. Each goroutine bumps bgJobs by 1 on
// entry and decrements on exit, so the host's Stop() path drains them.
func (s *Service) Start() {
	s.bgJobs.Add(1)
	go s.firmwareSchedulerWithRecover()
	s.bgJobs.Add(1)
	go s.auditRetentionLoop()
	s.bgJobs.Add(1)
	go s.autoBackupLoop()
	s.bgJobs.Add(1)
	go s.sessionSweepLoop()
}

// sessionSweepLoop deletes session rows whose expires_at has passed. S5 —
// without this the table grows unboundedly because Logout flips
// revoked_at but doesn't DELETE. The sweeper runs every 6h; sessions have
// a 7-day max lifetime so a 6h slack is invisible to operators.
func (s *Service) sessionSweepLoop() {
	defer s.bgJobs.Done()
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	// Immediate run on startup so an interrupt during a previous container
	// life does not leave a stale row visible until the first 6h tick.
	s.runSessionSweepOnce()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			s.runSessionSweepOnce()
		}
	}
}

func (s *Service) runSessionSweepOnce() {
	defer func() {
		if r := recover(); r != nil {
			s.logf(s.ctx, "ERROR", fmt.Sprintf("session sweep panic: %v", r))
		}
	}()
	n, err := s.store.PruneExpiredSessions()
	if err != nil {
		s.logf(s.ctx, "ERROR", fmt.Sprintf("session sweep: %v", err))
		return
	}
	if n > 0 {
		s.logf(s.ctx, "INFO", fmt.Sprintf("session sweep: pruned %d expired rows", n))
	}
}

// auditRetentionLoop is the hourly tick that reads AuditRetentionDays from
// AppSettings and calls PruneAuditLogOlderThan with the resulting cutoff.
// A retention of 0 disables pruning entirely (rows kept indefinitely).
func (s *Service) auditRetentionLoop() {
	defer s.bgJobs.Done()
	t := time.NewTicker(AuditRetentionTick)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			s.runAuditRetentionOnce()
		}
	}
}

func (s *Service) runAuditRetentionOnce() {
	defer func() {
		if r := recover(); r != nil {
			s.logf(s.ctx, "ERROR", fmt.Sprintf("audit retention pruner panic: %v", r))
		}
	}()
	settings, err := s.store.GetSettings()
	if err != nil {
		s.logf(s.ctx, "ERROR", fmt.Sprintf("audit retention: read settings: %v", err))
		return
	}
	if settings.AuditRetentionDays <= 0 {
		return
	}
	cutoff := time.Now().UTC().Add(-time.Duration(settings.AuditRetentionDays) * 24 * time.Hour)
	n, err := s.store.PruneAuditLogOlderThan(cutoff)
	if err != nil {
		s.logf(s.ctx, "ERROR", fmt.Sprintf("audit retention: prune failed: %v", err))
		return
	}
	if n > 0 {
		s.logf(s.ctx, "INFO", fmt.Sprintf("audit retention: pruned %d rows older than %s", n, cutoff.Format(time.RFC3339)))
	}
}

// autoBackupLoop runs the SQLite snapshot job at the operator-configured
// cadence (S12+S13). Ticks every minute and consults the latest settings
// each time so changing AutoBackupIntervalHours via the UI applies on the
// next tick without a service restart.
func (s *Service) autoBackupLoop() {
	defer s.bgJobs.Done()
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	var lastRun time.Time
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			settings, err := s.store.GetSettings()
			if err != nil {
				continue
			}
			if !settings.AutoBackupEnabled {
				continue
			}
			interval := time.Duration(settings.AutoBackupIntervalHours) * time.Hour
			if !lastRun.IsZero() && time.Since(lastRun) < interval {
				continue
			}
			if err := s.runAutoBackupOnce(settings); err != nil {
				s.logf(s.ctx, "ERROR", fmt.Sprintf("auto-backup: %v", err))
				continue
			}
			lastRun = time.Now()
		}
	}
}

func (s *Service) runAutoBackupOnce(settings models.AppSettings) error {
	stamp := time.Now().UTC().Format("20060102-150405")
	path := filepath.Join(s.dataDir, fmt.Sprintf("shellyctl.db.snap-%s.sqlite", stamp))
	if err := s.store.SnapshotTo(path); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	s.logf(s.ctx, "INFO", fmt.Sprintf("auto-backup: wrote %s", path))
	pattern := filepath.Join(s.dataDir, "shellyctl.db.snap-*.sqlite")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob: %w", err)
	}
	if len(matches) <= settings.AutoBackupKeep {
		return nil
	}
	// Filenames embed UTC timestamp in fixed-width format → lexical sort
	// equals chronological. Newest at end; keep the tail.
	sort.Strings(matches)
	for _, old := range matches[:len(matches)-settings.AutoBackupKeep] {
		if err := os.Remove(old); err != nil {
			s.logf(s.ctx, "WARN", fmt.Sprintf("auto-backup: prune %s failed: %v", old, err))
			continue
		}
	}
	return nil
}

// firmwareSchedulerWithRecover wraps the firmware-check scheduler so a
// single panic doesn't leave the service silently without periodic
// checks (S9). Restart is throttled: if the scheduler panics every <5s,
// we give up to avoid a hot loop.
func (s *Service) firmwareSchedulerWithRecover() {
	defer s.bgJobs.Done()
	const minLifetime = 5 * time.Second
	for {
		startedAt := time.Now()
		func() {
			defer func() {
				if r := recover(); r != nil {
					msg := fmt.Sprintf("firmware-check scheduler panic: %v", r)
					s.logf(s.ctx, "ERROR", msg)
				}
			}()
			s.runFirmwareScheduler()
		}()
		// Clean exit (ctx cancelled): leave the loop.
		if s.ctx.Err() != nil {
			return
		}
		// Crash-loop guard.
		if time.Since(startedAt) < minLifetime {
			s.logf(s.ctx, "ERROR", "firmware-check scheduler crashed twice in <5s, giving up; restart container to recover")
			return
		}
	}
}
