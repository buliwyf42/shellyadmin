-- Add cipher columns alongside the plaintext password / ha1 columns. For one
-- release the schema holds both: rows written before the upgrade still read
-- from plaintext, rows written after the upgrade populate the cipher columns
-- (and a one-shot sweep at startup migrates existing rows in place). A later
-- migration will drop the plaintext columns once every deployment has rotated
-- through at least once.
ALTER TABLE credentials ADD COLUMN password_cipher TEXT NOT NULL DEFAULT '';
ALTER TABLE credentials ADD COLUMN ha1_cipher TEXT NOT NULL DEFAULT '';
ALTER TABLE credential_groups ADD COLUMN password_cipher TEXT NOT NULL DEFAULT '';
ALTER TABLE credential_groups ADD COLUMN ha1_cipher TEXT NOT NULL DEFAULT '';
