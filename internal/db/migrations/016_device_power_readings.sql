-- Live power telemetry surfaced from EM/EM1/PM1/Switch component status.
-- Nullable; zero is a valid reading (e.g. switch off), so a NULL means
-- "device exposes no power telemetry at all".
ALTER TABLE devices ADD COLUMN power_w REAL;
ALTER TABLE devices ADD COLUMN voltage_v REAL;
ALTER TABLE devices ADD COLUMN current_a REAL;
