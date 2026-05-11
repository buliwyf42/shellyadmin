# ADR-0013: Encryption Key Externalization Requirement

- Status: `Accepted`
- Date: 2026-05-11
- Implements: v0.2.11 (deprecation warning) → v0.3.0 (hard-fail)
- Related: ADR-0006 (Backup, Export/Import, Secrets), ADR-0012 (STRIDE)

## Context

`internal/core/secretbox` encrypts credential password / HA1 fields
and the MCP token at rest. The 32-byte key is loaded at startup
through `loadEncryptionKey()` in `cmd/shellyctl/main.go`:

1. `SHELLYADMIN_ENCRYPTION_KEY` (env, base64) — operator-managed.
2. `SHELLYADMIN_ENCRYPTION_KEY_FILE` — same payload, file path.
3. `{dataDir}/shellyadmin.key` — auto-generated fallback.

The third path is convenient for a fresh setup but defeats the
threat model. The credential ciphertext sits in `shellyctl.db` on
the data volume; if the auto-generated key sits next to it, a
volume snapshot (operator backup, container escape, host backup
exfil) takes both. The consolidated review (F2 / S6) called this
"encryption at rest in name only."

## Decision

**v0.2.11 (shipped):** emit a stderr deprecation warning on every
startup that falls through to the auto-generation path. Tells
operators to migrate before v0.3.0.

**v0.3.0 (planned):** remove the auto-generation path. Missing
`SHELLYADMIN_ENCRYPTION_KEY` *and* `SHELLYADMIN_ENCRYPTION_KEY_FILE`
hard-fails at startup with a message pointing at the migration
recipe. Operators must place the key on a different volume than
the database — or any path the operator's own backup strategy
covers separately.

**Future (Phase 4+):** optional HSM/PKCS11 key provider behind a
`SHELLYADMIN_ENCRYPTION_KEY_PROVIDER=pkcs11` flag (ADR follow-up,
T4 in the consolidated review).

### Migration Recipe (operator-facing)

1. Existing v0.2.x deployment using auto-generated key:
   ```bash
   docker exec shellyadmin cat /data/shellyadmin.key
   ```
2. Copy the file to a path on the host the data volume does NOT
   cover. Recommended: `/etc/shellyadmin/encryption.key` on the
   host, or a separate Docker secret.
3. Update compose `.env`:
   ```
   SHELLYADMIN_ENCRYPTION_KEY_FILE=/run/secrets/shellyadmin_encryption_key
   ```
4. Mount the new path into the container:
   ```yaml
   secrets:
     - shellyadmin_encryption_key
   ```
5. Restart. The auto-generated `shellyadmin.key` inside `/data`
   becomes redundant; delete it during the next data-volume
   maintenance window so it doesn't get backed up alongside the
   DB.

### Why a Two-Version Deprecation Window

Operators who pulled v0.2.x without reading the docs got the
auto-generated key. Pulling v0.3.0 and finding the container
refuses to start with no migration warning would be a hostile
upgrade experience. v0.2.11's stderr warning + this ADR + the
CHANGELOG note give the diligent operator one full release to
prepare; the inattentive operator sees the warning every restart
and has time to act.

## Consequences

**Positive**

- Volume-snapshot exfil no longer compromises the credentials
  at rest. The key file lives on a different storage volume
  the operator can back up under a different access pattern.
- The encryption guarantee documented in ADR-0006 becomes real
  rather than aspirational.

**Negative**

- Operators have to do a one-time migration before v0.3.0.
- Key loss is now an operator responsibility — if the
  externalised key file is deleted and not backed up, all
  encrypted credentials are unrecoverable. The auto-generation
  fallback used to be a sort-of automatic backup (also on the
  data volume), at the cost of making the encryption useless.

**Mitigations**

- DEPLOYMENT.md gets a dedicated "Encryption Key Backup" section
  warning operators that the key must be backed up separately
  from the database.
- The hard-fail error message includes both the file path being
  checked and a one-line link to this ADR for the migration
  recipe.

## Related Work

- v0.2.11 commit 442784b emits the deprecation warning in
  `cmd/shellyctl/main.go`.
- ADR-0006 §"Encryption at Rest" — pre-existing description of
  how `secretbox` is used; this ADR upgrades the operational
  requirements.
- ADR-0012 §"Information Disclosure" — the threat model row this
  ADR closes.
