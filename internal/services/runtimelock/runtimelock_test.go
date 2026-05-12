package runtimelock

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"shellyadmin/internal/db"
)

// fakeStore is an in-memory Store keyed on PrimaryKey. Mirrors the
// runtime_locks row shape so the acquire/release state machine can be
// exercised without a real SQLite handle.
type fakeStore struct {
	mu  sync.Mutex
	row *db.RuntimeLockRow
}

func newFakeStore() *fakeStore { return &fakeStore{} }

func (f *fakeStore) GetRuntimeLock(key string) (db.RuntimeLockRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.row == nil || f.row.Key != key {
		return db.RuntimeLockRow{}, sql.ErrNoRows
	}
	return *f.row, nil
}

func (f *fakeStore) UpsertRuntimeLock(row db.RuntimeLockRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy := row
	f.row = &copy
	return nil
}

func (f *fakeStore) DeleteRuntimeLock(string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.row = nil
	return nil
}

func (f *fakeStore) ForceClearRuntimeLock(string) error {
	return f.DeleteRuntimeLock("")
}

// TestAcquireSucceedsOnEmptyTable locks in the boot-time happy path: a
// fresh DB with no row → Acquire claims it and Release tears it down.
func TestAcquireSucceedsOnEmptyTable(t *testing.T) {
	store := newFakeStore()
	svc := New(store)

	if err := svc.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire on empty table: %v", err)
	}
	if store.row == nil {
		t.Fatalf("Acquire did not write a row")
	}
	if store.row.Key != PrimaryKey {
		t.Errorf("Key = %q, want %q", store.row.Key, PrimaryKey)
	}
	if err := svc.Release(context.Background()); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if store.row != nil {
		t.Errorf("Release did not delete the row")
	}
}

// TestAcquireRefusesFreshForeignLock locks in the ADR-0015 contract:
// when another instance's row is fresher than StaleAfter, Acquire
// returns ErrLocked carrying the foreign instance's metadata.
func TestAcquireRefusesFreshForeignLock(t *testing.T) {
	store := newFakeStore()
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	// Pre-seed a foreign row, 30s old.
	store.row = &db.RuntimeLockRow{
		Key:        PrimaryKey,
		InstanceID: "foreign-instance",
		AcquiredAt: now.Add(-30 * time.Second).Format(time.RFC3339),
		PID:        9999,
		Hostname:   "other-host",
	}

	svc := New(store)
	svc.SetClock(func() time.Time { return now })

	err := svc.Acquire(context.Background())
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("Acquire: got %v, want ErrLocked", err)
	}
	var le *LockedError
	if !errors.As(err, &le) {
		t.Fatalf("Acquire error is not a LockedError: %T", err)
	}
	if le.Hostname != "other-host" {
		t.Errorf("Hostname = %q, want %q", le.Hostname, "other-host")
	}
	if le.PID != 9999 {
		t.Errorf("PID = %d, want %d", le.PID, 9999)
	}
	// We must NOT have overwritten the row.
	if store.row.Hostname != "other-host" {
		t.Errorf("Acquire mutated foreign row: %+v", store.row)
	}
}

// TestAcquireTakesOverStaleLock locks in the kill-9 recovery path: a
// row older than StaleAfter is silently overwritten.
func TestAcquireTakesOverStaleLock(t *testing.T) {
	store := newFakeStore()
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	// Pre-seed a stale row (1 hour old; StaleAfter is 5 minutes).
	store.row = &db.RuntimeLockRow{
		Key:        PrimaryKey,
		InstanceID: "dead-instance",
		AcquiredAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
		PID:        12345,
		Hostname:   "kill-9d-host",
	}

	svc := New(store)
	svc.SetClock(func() time.Time { return now })

	if err := svc.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire over stale: %v", err)
	}
	if store.row.Hostname == "kill-9d-host" {
		t.Errorf("Acquire did not overwrite stale row")
	}
	if store.row.InstanceID == "dead-instance" {
		t.Errorf("Acquire did not refresh instance_id")
	}
}

// TestAcquireIsIdempotent — calling Acquire twice on the same service
// (e.g. a defensive double-call on shutdown-handler recovery) must
// not error and must not double-write.
func TestAcquireIsIdempotent(t *testing.T) {
	store := newFakeStore()
	svc := New(store)
	if err := svc.Acquire(context.Background()); err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	if err := svc.Acquire(context.Background()); err != nil {
		t.Errorf("second Acquire: %v", err)
	}
}

// TestReleaseIsSafeBeforeAcquire — Release without Acquire is a no-op,
// not an error. Protects shutdown paths that defer Release before
// checking whether Acquire actually ran.
func TestReleaseIsSafeBeforeAcquire(t *testing.T) {
	svc := New(newFakeStore())
	if err := svc.Release(context.Background()); err != nil {
		t.Errorf("Release before Acquire: %v", err)
	}
}

// TestAcquireTakesOverOwnHostname locks in the v0.3.3 fast path:
// when the existing row's hostname matches the current process's
// hostname (i.e. the same Docker container restarting itself after a
// crash), Acquire takes over the row immediately without waiting for
// the staleness window. This is the crash-restart-loop case that
// dominated operator pain on v0.3.0–0.3.2 deployments.
func TestAcquireTakesOverOwnHostname(t *testing.T) {
	store := newFakeStore()
	myHostname, _ := os.Hostname()
	if myHostname == "" {
		t.Skip("test host has no hostname")
	}
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	// Pre-seed a row with OUR hostname, only 10s old (well within
	// the 60s staleness window). A pre-v0.3.3 service would refuse;
	// the new fast path takes over.
	store.row = &db.RuntimeLockRow{
		Key:        PrimaryKey,
		InstanceID: "previous-boot-of-same-container",
		AcquiredAt: now.Add(-10 * time.Second).Format(time.RFC3339),
		PID:        7,
		Hostname:   myHostname,
	}

	svc := New(store)
	svc.SetClock(func() time.Time { return now })

	if err := svc.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire with own hostname: %v (want takeover)", err)
	}
	if store.row.InstanceID == "previous-boot-of-same-container" {
		t.Errorf("Acquire did not overwrite own-hostname row: %+v", store.row)
	}
}

// TestStaleAfterIs60Seconds locks in the v0.3.3 window-tightening.
// A future change that lifts this back to 5min would re-introduce
// the crash-restart pain v0.3.3 was cut to fix.
func TestStaleAfterIs60Seconds(t *testing.T) {
	if StaleAfter != 60*time.Second {
		t.Errorf("StaleAfter = %v, want 60s (v0.3.3 lock-window tightening)", StaleAfter)
	}
}

// TestForceClearRemovesRow locks in the `shellyctl unlock --force`
// path: an active row is unconditionally deleted, regardless of
// acquired_at.
func TestForceClearRemovesRow(t *testing.T) {
	store := newFakeStore()
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	store.row = &db.RuntimeLockRow{
		Key:        PrimaryKey,
		InstanceID: "live-instance",
		AcquiredAt: now.Format(time.RFC3339), // fresh!
		PID:        1,
		Hostname:   "active-host",
	}
	if err := ForceClear(store); err != nil {
		t.Fatalf("ForceClear: %v", err)
	}
	if store.row != nil {
		t.Errorf("ForceClear did not delete the live row")
	}
}
