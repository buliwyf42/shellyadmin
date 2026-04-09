CREATE TABLE IF NOT EXISTS credential_groups (
  name TEXT PRIMARY KEY,
  credential_ref TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS device_credential_groups (
  mac TEXT PRIMARY KEY,
  group_name TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
