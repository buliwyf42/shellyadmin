ALTER TABLE devices ADD COLUMN last_refresh_attempt TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN last_refresh_ok INTEGER NOT NULL DEFAULT 1;
ALTER TABLE devices ADD COLUMN last_refresh_error TEXT NOT NULL DEFAULT '';

UPDATE devices
SET last_refresh_attempt = COALESCE(last_seen, ''),
    last_refresh_ok = CASE WHEN COALESCE(last_seen, '') = '' THEN 0 ELSE 1 END,
    last_refresh_error = '';
