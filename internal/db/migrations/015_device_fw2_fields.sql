-- Adds device columns introduced for Shelly firmware 2.0.0-beta1 compatibility:
--   scheme              http or https (learned at probe time)
--   enhanced_security   tri-state mirror of device's enhanced_security flag
--   tls_cert_valid      result of HTTPS cert date/chain validation
--   tls_allow_insecure  per-device opt-out of TLS verification
--   auth_locked_until   ISO timestamp populated when device returns 429
--   wifi_hostname       hostname configured on the device
--   wifi_channel        channel number from wifi.sta_status
ALTER TABLE devices ADD COLUMN scheme TEXT NOT NULL DEFAULT 'http';
ALTER TABLE devices ADD COLUMN enhanced_security INTEGER;
ALTER TABLE devices ADD COLUMN tls_cert_valid INTEGER;
ALTER TABLE devices ADD COLUMN tls_allow_insecure INTEGER;
ALTER TABLE devices ADD COLUMN auth_locked_until TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN wifi_hostname TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN wifi_channel INTEGER NOT NULL DEFAULT 0;
