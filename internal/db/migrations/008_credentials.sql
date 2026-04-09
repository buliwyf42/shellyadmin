CREATE TABLE IF NOT EXISTS credentials (
  name TEXT PRIMARY KEY,
  username TEXT NOT NULL,
  password TEXT NOT NULL DEFAULT '',
  ha1 TEXT NOT NULL DEFAULT '',
  tags TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

ALTER TABLE templates ADD COLUMN credential_ref TEXT NOT NULL DEFAULT '';
