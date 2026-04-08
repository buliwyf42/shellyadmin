CREATE TABLE IF NOT EXISTS devices (
  mac TEXT PRIMARY KEY,
  ip TEXT,
  name TEXT,
  model TEXT,
  fw TEXT,
  gen INTEGER,
  online INTEGER,
  last_seen TEXT,
  first_seen TEXT,
  device_num INTEGER,
  consecutive_misses INTEGER,
  mqtt_enabled INTEGER,
  mqtt_server TEXT,
  mqtt_client_id TEXT,
  mqtt_topic_prefix TEXT,
  mqtt_flags_na TEXT,
  lat REAL,
  lon REAL,
  tz TEXT,
  ws_enabled INTEGER,
  ws_server TEXT,
  ble_gw_enabled INTEGER,
  wifi_ssid TEXT,
  fw_status TEXT,
  fw_available_ver TEXT,
  cloud_enabled INTEGER,
  cloud_connected INTEGER,
  ws_connected INTEGER,
  matter_enabled INTEGER,
  time_format TEXT,
  sntp_server TEXT,
  serial TEXT,
  eco_mode INTEGER,
  discoverable INTEGER
);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS templates (
  name TEXT PRIMARY KEY,
  content TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts TEXT NOT NULL,
  level TEXT NOT NULL,
  message TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS jobs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL,
  status TEXT NOT NULL,
  done INTEGER NOT NULL DEFAULT 0,
  total INTEGER NOT NULL DEFAULT 0,
  payload TEXT NOT NULL DEFAULT '',
  result TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
