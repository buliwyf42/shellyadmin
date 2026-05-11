-- 029_personal_access_tokens.sql — T3 from the consolidated review.
-- Bearer-token credentials for headless callers (Home Assistant, cron
-- jobs, scripts) so /api/* mutations don't have to fake the cookie +
-- CSRF dance. Each row stores the argon2id PHC of the bearer token —
-- the plaintext is shown to the operator exactly once at creation,
-- never persisted.
--
-- Token format on the wire: `pat_<id>_<random>` where <id> is the row's
-- 8-hex-char id and <random> is 32 bytes hex. Lookups hit the id column
-- (constant-time index miss is OK; a malformed token returns the same
-- 401 shape so the response timing is independent of id existence).
CREATE TABLE IF NOT EXISTS personal_access_tokens (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    scopes TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_used_at TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL DEFAULT '',
    revoked_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_pat_username ON personal_access_tokens(username);
