-- Drop the pre-v0.1.5 single-channel firmware columns. The v0.1.5 rebuild
-- (migration 017) replaced them with the per-channel cache
-- (fw_available_stable, fw_available_beta, fw_checked_at) and Go code
-- stopped reading or writing the legacy columns at that release. Keeping
-- them around preserved the rollback path for one release window; that
-- window is closed as of v0.1.7.
--
-- SQLite gained ALTER TABLE DROP COLUMN in 3.35 (March 2021). The
-- modernc.org/sqlite driver this project uses is more recent; the
-- operation is in-place and does not require the rebuild-table dance.
ALTER TABLE devices DROP COLUMN fw_status;
ALTER TABLE devices DROP COLUMN fw_available_ver;
