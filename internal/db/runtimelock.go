package db

// Single-instance runtime lock rows (ADR-0015). MOVED FROM db.go —
// db-layer split by domain (post-v0.5.2 review item 6); bodies unchanged.

// RuntimeLockRow mirrors the runtime_locks table for the runtimelock
// package's Store interface. The package doesn't import db directly;
// db's methods accept + return the shared shape.
type RuntimeLockRow struct {
	Key        string
	InstanceID string
	AcquiredAt string
	PID        int
	Hostname   string
}

// GetRuntimeLock returns the row for key, or sql.ErrNoRows when no
// row exists. Used at startup to decide whether to refuse the boot.
func (db *DB) GetRuntimeLock(key string) (RuntimeLockRow, error) {
	var r RuntimeLockRow
	err := db.sql.QueryRow(
		`SELECT key, instance_id, acquired_at, pid, hostname
		 FROM runtime_locks WHERE key = ?`,
		key,
	).Scan(&r.Key, &r.InstanceID, &r.AcquiredAt, &r.PID, &r.Hostname)
	return r, err
}

// UpsertRuntimeLock writes the row, overwriting any existing entry
// for the key. Used by Acquire (boot-time claim, may overwrite a
// stale row) and by the heartbeat (bump acquired_at every minute).
func (db *DB) UpsertRuntimeLock(row RuntimeLockRow) error {
	_, err := db.sql.Exec(
		`INSERT INTO runtime_locks(key, instance_id, acquired_at, pid, hostname)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET
			instance_id = excluded.instance_id,
			acquired_at = excluded.acquired_at,
			pid = excluded.pid,
			hostname = excluded.hostname`,
		row.Key, row.InstanceID, row.AcquiredAt, row.PID, row.Hostname,
	)
	return err
}

// DeleteRuntimeLock removes the row keyed by `key`. Used by Release
// on graceful shutdown. Best-effort: a failure here leaves a row
// that'll go stale within ~5 minutes and be overwritten on the next
// startup, so we never block shutdown on this.
func (db *DB) DeleteRuntimeLock(key string) error {
	_, err := db.sql.Exec(`DELETE FROM runtime_locks WHERE key = ?`, key)
	return err
}

// ForceClearRuntimeLock unconditionally removes the row regardless
// of acquired_at. The `shellyctl unlock --force` subcommand calls
// this to recover from a wedged previous container without waiting
// for the staleness window.
func (db *DB) ForceClearRuntimeLock(key string) error {
	_, err := db.sql.Exec(`DELETE FROM runtime_locks WHERE key = ?`, key)
	return err
}
