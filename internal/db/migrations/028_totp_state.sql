-- 028_totp_state.sql — T1 from the consolidated review. Per-user TOTP
-- second-factor state: the shared secret + backup-codes envelope. The
-- secret_cipher and backup_codes_cipher columns hold secretbox-sealed
-- payloads (see internal/core/secretbox) so a compromised DB file does
-- not leak the seed to a passive attacker — the encryption key must be
-- in the environment (SHELLYADMIN_ENCRYPTION_KEY / _FILE) to read either.
--
-- Single-row-per-user keyed on username. Like login_state (025), this is
-- single-row-only today under ADR-0001 but keys the door open for
-- multi-user without a future migration.
CREATE TABLE IF NOT EXISTS totp_state (
    username TEXT PRIMARY KEY,
    secret_cipher TEXT NOT NULL,
    enrolled_at TEXT NOT NULL,
    last_verified_at TEXT NOT NULL DEFAULT '',
    backup_codes_cipher TEXT NOT NULL,
    backup_codes_used INTEGER NOT NULL DEFAULT 0
);
