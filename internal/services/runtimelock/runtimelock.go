// Package runtimelock enforces the single-instance invariant from
// ADR-0015. On startup the service writes the `runtime_locks` row keyed
// `primary` and starts a background heartbeat. A second container
// booting against the same SQLite file finds a fresh row and refuses
// to start; a stale row (5+ minutes without heartbeat — covers
// kill-9'd previous container) is taken over automatically.
//
// Why this exists: ShellyAdmin's process-local state (login rate-limit
// map, MCP listener, background workers) does NOT replicate across
// instances. Two containers reading the same DB would issue duplicate
// firmware checks, race the audit-log retention transaction, and try
// to bind the same MCP port. The runtime_locks row is the explicit
// door-closer documented in ADR-0015.
package runtimelock

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"shellyadmin/internal/db"
)

const (
	// PrimaryKey is the single key written today. Future per-feature
	// locks ("only one firmware-install in flight per device") can
	// reuse the same table with different key strings.
	PrimaryKey = "primary"

	// HeartbeatInterval is how often the live owner bumps acquired_at
	// so a passive observer can tell "still alive" from "dead and
	// abandoned". 60s matches the ADR-0015 description.
	HeartbeatInterval = 60 * time.Second

	// StaleAfter is the cutoff a passive observer uses. 5 minutes = 5×
	// the heartbeat interval, giving slack for GC pauses + clock
	// skew + a slow snapshot pause.
	StaleAfter = 5 * time.Minute
)

// ErrLocked is returned by Acquire when the table already has a fresh
// `primary` row owned by another instance. The detail string in the
// returned LockedError names the foreign hostname/pid + when the row
// will go stale, so the operator's `docker logs` line is actionable.
var ErrLocked = errors.New("runtimelock: primary lock held by another instance")

// LockedError carries the foreign owner's metadata. Callers can switch
// on errors.As to render a richer message than the bare Error() text.
type LockedError struct {
	Hostname   string
	PID        int
	AcquiredAt string
	StaleAfter time.Time
}

func (e *LockedError) Error() string {
	return fmt.Sprintf(
		"runtimelock: primary lock held by another instance — hostname=%q pid=%d acquired_at=%s "+
			"(stale after %s)",
		e.Hostname, e.PID, e.AcquiredAt, e.StaleAfter.UTC().Format(time.RFC3339),
	)
}

func (e *LockedError) Is(target error) bool { return target == ErrLocked }

// Store is the narrow persistence surface. *db.DB satisfies it
// structurally via the methods added in the 030 migration; tests can
// substitute a fake without standing up a real SQLite handle.
type Store interface {
	GetRuntimeLock(key string) (db.RuntimeLockRow, error)
	UpsertRuntimeLock(row db.RuntimeLockRow) error
	DeleteRuntimeLock(key string) error
	// ForceClearRuntimeLock unconditionally removes the row,
	// regardless of acquired_at freshness. Used by `shellyctl unlock
	// --force` for the rare case where the operator wants to recover
	// from a wedged previous container without waiting for the
	// staleness window.
	ForceClearRuntimeLock(key string) error
}

// Service owns the lifecycle: Acquire (boot-time), Heartbeat (ticker
// goroutine), Release (shutdown).
type Service struct {
	store      Store
	instanceID string
	hostname   string
	pid        int

	now func() time.Time

	mu       sync.Mutex
	held     bool
	stopBeat chan struct{}
	beatDone chan struct{}
}

// New constructs a Service. The instanceID is a fresh random hex
// string so two containers with the same hostname (e.g. both
// "shellyadmin" in different docker compose stacks against the same
// volume — operator misconfiguration but possible) still produce
// distinguishable ownership.
func New(store Store) *Service {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		// rand.Read failure is catastrophic + extremely rare; the
		// fallback ensures a unique-enough id even then.
		idBytes = []byte(time.Now().UTC().Format("20060102T150405.000000"))
	}
	return &Service{
		store:      store,
		instanceID: hex.EncodeToString(idBytes),
		hostname:   hostname,
		pid:        os.Getpid(),
	}
}

// SetClock overrides the wall-clock source. Used by tests; production
// never calls this.
func (s *Service) SetClock(fn func() time.Time) {
	s.now = fn
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now().UTC()
}

// Acquire claims the primary lock. Returns ErrLocked (wrapped in a
// LockedError carrying owner metadata) when the table has a fresh row
// owned by someone else. A stale row is overwritten — the previous
// owner died without releasing.
func (s *Service) Acquire(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held {
		return nil
	}
	now := s.clock().UTC()
	existing, err := s.store.GetRuntimeLock(PrimaryKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("runtimelock: read existing lock: %w", err)
	}
	if err == nil && existing.InstanceID != "" {
		acquiredAt, parseErr := time.Parse(time.RFC3339, existing.AcquiredAt)
		if parseErr == nil && now.Sub(acquiredAt) < StaleAfter {
			return &LockedError{
				Hostname:   existing.Hostname,
				PID:        existing.PID,
				AcquiredAt: existing.AcquiredAt,
				StaleAfter: acquiredAt.Add(StaleAfter),
			}
		}
		// Stale row — log path is the caller's; we take it over.
	}
	row := db.RuntimeLockRow{
		Key:        PrimaryKey,
		InstanceID: s.instanceID,
		AcquiredAt: now.Format(time.RFC3339),
		PID:        s.pid,
		Hostname:   s.hostname,
	}
	if err := s.store.UpsertRuntimeLock(row); err != nil {
		return fmt.Errorf("runtimelock: claim lock: %w", err)
	}
	s.held = true
	return nil
}

// StartHeartbeat spawns a ticker goroutine that bumps acquired_at on
// the row every HeartbeatInterval until Release is called. The ticker
// exits cleanly on Release; ctx cancellation is also honored so a
// parent shutdown can stop the heartbeat early.
func (s *Service) StartHeartbeat(ctx context.Context, onError func(error)) {
	s.mu.Lock()
	if !s.held || s.stopBeat != nil {
		s.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	s.stopBeat = stop
	s.beatDone = done
	s.mu.Unlock()

	go func() {
		defer close(done)
		ticker := time.NewTicker(HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				row := db.RuntimeLockRow{
					Key:        PrimaryKey,
					InstanceID: s.instanceID,
					AcquiredAt: s.clock().UTC().Format(time.RFC3339),
					PID:        s.pid,
					Hostname:   s.hostname,
				}
				if err := s.store.UpsertRuntimeLock(row); err != nil {
					if onError != nil {
						onError(fmt.Errorf("runtimelock: heartbeat: %w", err))
					}
				}
			}
		}
	}()
}

// Release deletes the row (if owned) and stops the heartbeat. Safe to
// call without Acquire; safe to call twice.
func (s *Service) Release(ctx context.Context) error {
	s.mu.Lock()
	if !s.held {
		s.mu.Unlock()
		return nil
	}
	stop := s.stopBeat
	done := s.beatDone
	s.stopBeat = nil
	s.beatDone = nil
	s.held = false
	s.mu.Unlock()

	if stop != nil {
		close(stop)
	}
	if done != nil {
		// Bounded wait — a stuck heartbeat goroutine should not block
		// shutdown indefinitely.
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
		}
	}

	// Best-effort delete. If it fails, the row's acquired_at will go
	// stale and the next startup will overwrite it — we never block
	// shutdown on a delete error.
	if err := s.store.DeleteRuntimeLock(PrimaryKey); err != nil {
		return fmt.Errorf("runtimelock: release: %w", err)
	}
	return nil
}

// ForceClear is the `shellyctl unlock --force` path. Removes the row
// regardless of acquired_at freshness. Used when an operator knows the
// previous container died and doesn't want to wait the 5-minute
// staleness window.
func ForceClear(store Store) error {
	return store.ForceClearRuntimeLock(PrimaryKey)
}
