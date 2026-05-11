-- 027_server_sessions.sql — S5 from the consolidated review. The
-- cookie-only session store (gin-contrib/sessions + cookie.NewStore)
-- is HMAC-signed but stateless: a leaked cookie remains valid for its
-- full MaxAge (7 days) even after the operator notices and signs out.
-- This table is the server-side anchor: each Login row issues a row,
-- Logout flips revoked_at, and RequireAuth refuses any cookie whose
-- session_id is missing, expired, or revoked.
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT NOT NULL DEFAULT ''
);

-- Hot path: RequireAuth looks up by id. Cold paths (operator audit,
-- bulk revoke on password change) scan by username or by status —
-- the table is small enough (one row per operator login) that a
-- single index on (id) suffices.
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
