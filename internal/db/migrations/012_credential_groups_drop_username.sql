DROP TABLE IF EXISTS credential_groups_v3;

CREATE TABLE credential_groups_v3 (
  name TEXT PRIMARY KEY,
  credential_ref TEXT NOT NULL,
  password TEXT NOT NULL DEFAULT '',
  ha1 TEXT NOT NULL DEFAULT '',
  tags TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

INSERT INTO credential_groups_v3(name, credential_ref, password, ha1, tags, created_at, updated_at)
SELECT
  name,
  COALESCE(NULLIF(credential_ref, ''), name),
  password,
  ha1,
  tags,
  created_at,
  updated_at
FROM credential_groups;

DROP TABLE credential_groups;
ALTER TABLE credential_groups_v3 RENAME TO credential_groups;

UPDATE credentials
SET password = COALESCE((SELECT password FROM credential_groups WHERE credential_groups.name = credentials.name), password),
    ha1 = COALESCE((SELECT ha1 FROM credential_groups WHERE credential_groups.name = credentials.name), ha1),
    tags = COALESCE((SELECT tags FROM credential_groups WHERE credential_groups.name = credentials.name), tags),
    updated_at = CURRENT_TIMESTAMP
WHERE name IN (SELECT name FROM credential_groups);
