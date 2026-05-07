-- Application code returned by Shelly's GET /shelly + Shelly.GetDeviceInfo
-- under the "app" key (e.g. "PlugSG3", "Pro4PM", "Switch1Mini"). Shorter
-- and friendlier than the canonical model SKU; the Devices and Firmware
-- pages use it as the primary "what is this device" label and demote the
-- raw model code into the hover tooltip.
ALTER TABLE devices ADD COLUMN app TEXT NOT NULL DEFAULT '';
