-- Drop the pre-v0.0.15 plaintext credential columns. Since v0.0.15 every
-- boot has run encryptPlaintextCredentials() at Open() time, sweeping any
-- non-empty plaintext into the cipher columns and zeroing the plaintext.
-- Practical state of these columns in any current install: empty strings.
--
-- Cipher columns (password_cipher, ha1_cipher) added in migration 013 are
-- the only source of truth from this release forward.
--
-- Note: SQLite ALTER TABLE DROP COLUMN does not VACUUM the file, so
-- previously-written plaintext bytes can remain on disk pages until the
-- pages are recycled. Operators with strict scrubbing requirements should
-- run `sqlite3 shellyctl.db "VACUUM"` once after upgrade. The Go-side
-- value is already zeroed; this is purely about page-level forensics.
ALTER TABLE credentials DROP COLUMN password;
ALTER TABLE credentials DROP COLUMN ha1;
ALTER TABLE credential_groups DROP COLUMN password;
ALTER TABLE credential_groups DROP COLUMN ha1;
