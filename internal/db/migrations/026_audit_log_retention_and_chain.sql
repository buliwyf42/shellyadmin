-- 026_audit_log_retention_and_chain.sql — S1 + S2 from the consolidated
-- review. Adds the tamper-evidence column (prev_hash, hex SHA-256 of the
-- previous row's serialised form) and the append-only trigger that
-- prevents in-process DELETE statements from rewriting history. The
-- retention job that prunes rows older than AuditRetentionDays bypasses
-- the trigger via PRAGMA recursive_triggers=off in a controlled scope
-- (services.PruneAuditLog) — direct DELETE attempts from anywhere else
-- (operator-MCP, future bulk_logs handler) will fail loudly.
--
-- prev_hash is empty on the bootstrap row inserted at first migration.
-- New rows fill it from the previous id at write time (see
-- internal/db/db.go AddLogWithAttrs). A verification tool walks the
-- table in ascending id order and recomputes the chain.
ALTER TABLE audit_log ADD COLUMN prev_hash TEXT NOT NULL DEFAULT '';

-- Append-only trigger. Anything that needs to actually delete rows
-- (the retention job) must temporarily set the trigger session-local
-- via `DROP TRIGGER` + `CREATE TRIGGER` in a transaction, or use the
-- raw SQLite handle. The trigger is the operator-facing protection
-- against ad-hoc `DELETE FROM audit_log WHERE ...` in a debug
-- session that would otherwise rewrite forensic history.
--
-- NOTE: SQLite triggers are stored at the schema level; if a future
-- migration changes audit_log's structure it MUST also re-issue this
-- trigger.
CREATE TRIGGER IF NOT EXISTS audit_log_no_delete
BEFORE DELETE ON audit_log
WHEN COALESCE((SELECT value FROM settings WHERE key = '__retention_bypass'), '0') != '1'
BEGIN
    SELECT RAISE(ABORT, 'audit_log is append-only; use the retention job (services.PruneAuditLog) to remove old rows');
END;
