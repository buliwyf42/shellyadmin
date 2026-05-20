-- 031_admin_credentials.sql — first-run setup (operator login in the DB).
-- Replaces the boot-time SHELLYADMIN_PASS_HASH / SHELLYADMIN_USER env vars
-- as the source of truth for the operator's login. The hash is an argon2id
-- PHC string (one-way), so it is stored as-is — NOT secretbox-sealed — and
-- lives in the same backup/rollback boundary as the rest of shellyctl.db.
--
-- Single-row table (CHECK id = 1): there is exactly one operator account
-- in the single-operator model. An empty table means "not configured yet"
-- and boots the server into setup mode.
--
-- This table is deliberately NOT part of AppSettings, which round-trips to
-- the SPA via GET /api/settings — a password hash must never reach that
-- surface.
CREATE TABLE IF NOT EXISTS admin_credentials (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    username   TEXT NOT NULL,
    pass_hash  TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
