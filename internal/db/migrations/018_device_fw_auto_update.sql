-- Auto-update mode read from each device's Schedule.List (the device's local
-- web UI implements its "Auto Update Firmware" toggle as a Schedule.Create
-- entry that calls Shelly.Update with origin="shelly_service"). Values:
--   ''        -> unknown / never read
--   'off'     -> no scheduled auto-update job
--   'stable'  -> scheduled auto-update on the stable channel
--   'beta'    -> scheduled auto-update on the beta channel
ALTER TABLE devices ADD COLUMN fw_auto_update TEXT NOT NULL DEFAULT '';
