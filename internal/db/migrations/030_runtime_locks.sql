-- 030_runtime_locks.sql — ADR-0015 (single-instance constraint).
-- Records the live primary lock so a second container starting against
-- the same SQLite file refuses to boot instead of producing subtle
-- duplicate-work bugs (firmware-check scheduler running twice, MCP
-- listener trying to re-bind :8081, etc.).
--
-- key='primary' is the only key written today. The schema is general
-- (key TEXT PRIMARY KEY) so future per-feature row-with-heartbeat
-- locks ("only one firmware-install job in flight per device") can
-- reuse the same table without another migration.
CREATE TABLE IF NOT EXISTS runtime_locks (
    key TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL,
    acquired_at TEXT NOT NULL,
    pid INTEGER NOT NULL,
    hostname TEXT NOT NULL
);
