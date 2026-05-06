-- Per-channel firmware availability cache.
-- Replaces the single fw_available_ver / fw_status columns: we now store the
-- latest stable and beta versions reported by Shelly.CheckForUpdate so the
-- channel selector on the Update page is purely a display filter (no re-check
-- needed when toggling). fw_checked_at lets the UI render "Checked X minutes
-- ago"; NULL means never checked. The legacy columns are kept (untouched) for
-- forward-compat across rollback windows; readers should ignore them.
ALTER TABLE devices ADD COLUMN fw_available_stable TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN fw_available_beta TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN fw_checked_at TEXT NOT NULL DEFAULT '';
