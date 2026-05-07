-- Two extra identifiers from Shelly.GetDeviceInfo / GET /shelly that are
-- useful for support and diagnostics:
--   batch   production batch label (e.g. "2430-Broadwell"). Tied to a
--           specific factory run; helpful when diagnosing model-specific
--           hardware quirks or filing warranty cases.
--   fw_id   full firmware identifier including build hash
--           (e.g. "20260423-102547/2.0.0-beta1-g8c7700a"). Distinct from
--           `fw` which is the friendly version string.
--
-- Both populated on every scan / refresh / firmware check; empty for
-- existing rows until then.
ALTER TABLE devices ADD COLUMN batch TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN fw_id TEXT NOT NULL DEFAULT '';
