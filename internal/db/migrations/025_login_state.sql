-- 025_login_state.sql — per-user login failure tracking + lockout (Phase 1 Q20).
-- Backs the account-lockout middleware: after N consecutive failed logins,
-- the username is locked for a cooldown window. Counter resets on a
-- successful login. Single-row-per-user; under ADR-0001 there is only ever
-- one row, but keying by username keeps the future option of multi-user
-- open without another migration.
CREATE TABLE IF NOT EXISTS login_state (
    username TEXT PRIMARY KEY,
    failed_count INTEGER NOT NULL DEFAULT 0,
    last_failed_at TEXT NOT NULL DEFAULT '',
    locked_until TEXT NOT NULL DEFAULT ''
);
