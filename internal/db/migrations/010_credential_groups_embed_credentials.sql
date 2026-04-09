DROP TABLE IF EXISTS credential_groups_v2;

CREATE TABLE credential_groups_v2 (
  name TEXT PRIMARY KEY,
  credential_ref TEXT NOT NULL,
  username TEXT NOT NULL DEFAULT 'admin',
  password TEXT NOT NULL DEFAULT '',
  ha1 TEXT NOT NULL DEFAULT '',
  tags TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

INSERT INTO credential_groups_v2(name, credential_ref, username, password, ha1, tags, created_at, updated_at)
SELECT
  name,
  COALESCE(NULLIF(credential_ref, ''), name),
  COALESCE((SELECT username FROM credentials WHERE credentials.name = credential_groups.credential_ref), 'admin'),
  COALESCE((SELECT password FROM credentials WHERE credentials.name = credential_groups.credential_ref), ''),
  COALESCE((SELECT ha1 FROM credentials WHERE credentials.name = credential_groups.credential_ref), ''),
  COALESCE((SELECT tags FROM credentials WHERE credentials.name = credential_groups.credential_ref), '[]'),
  created_at,
  updated_at
FROM credential_groups;

DROP TABLE credential_groups;
ALTER TABLE credential_groups_v2 RENAME TO credential_groups;

INSERT OR IGNORE INTO credentials(name, username, password, ha1, tags, created_at, updated_at)
SELECT name, username, password, ha1, tags, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM credential_groups;

UPDATE credentials
SET username = COALESCE((SELECT username FROM credential_groups WHERE credential_groups.name = credentials.name), username),
    password = COALESCE((SELECT password FROM credential_groups WHERE credential_groups.name = credentials.name), password),
    ha1 = COALESCE((SELECT ha1 FROM credential_groups WHERE credential_groups.name = credentials.name), ha1),
    tags = COALESCE((SELECT tags FROM credential_groups WHERE credential_groups.name = credentials.name), tags),
    updated_at = CURRENT_TIMESTAMP
WHERE name IN (SELECT name FROM credential_groups);
