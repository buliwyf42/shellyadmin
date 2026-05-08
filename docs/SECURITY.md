# Security Model

## Intended Environment

ShellyAdmin is designed for:

- trusted LAN environments
- a single trusted operator
- local network device administration

It is not designed for:

- direct internet exposure
- multi-tenant use
- public API access

## Authentication

- single admin login
- session-cookie authentication
- login rate limiting
- POST logout

### Admin password storage

- Preferred: set `SHELLYADMIN_PASS_HASH` (or `_FILE`) to an argon2id PHC string.
  Generate with `shellyctl hash-password <plaintext>`. Only the hash sits in
  env/memory at runtime — constant-time compared on login.
- Deprecated: `SHELLYADMIN_PASS` still accepts a plaintext password for
  existing deployments. Startup logs a deprecation warning. Plaintext support
  is scheduled for removal in **v0.2.0**, no earlier than **2026-07-22** —
  the 3-month overlap window from the v0.0.15 deprecation (2026-04-22). After
  removal, missing `SHELLYADMIN_PASS_HASH` will panic at startup with a
  pointer to `shellyctl hash-password`.

## Session Security

- signed cookies
- `Secure` cookies supported for proxied TLS deployments
- sessions should be considered LAN-admin sessions, not internet-facing auth tokens

## Request Handling

- request size limits are enforced on JSON endpoints
- mutating operations require authenticated session access
- delete actions require explicit confirmation in UI
- preview coverage for risky actions is still being expanded

## Logging

Operator-facing logging is audit events in SQLite:

- stored in SQLite
- meaningful actions only
- intended as the in-app log surface

Secrets should never be written to logs in raw form.

## Provisioning Secrets

Current product decision:

- templates may contain real secrets
- auth groups may contain real secrets (`password` and/or `ha1`)

Required safeguards:

- secret-bearing templates should be clearly marked in the UI
- secret-bearing auth groups should be clearly marked in the UI
- exports should redact secret values by default
- password-derived device auth material should remain ephemeral during execution

### Encryption at Rest

Device credential `password` and `ha1` values stored in `credentials` and
`credential_groups` are encrypted with XSalsa20-Poly1305 (NaCl secretbox). The
32-byte key is resolved in this order at startup:

1. `SHELLYADMIN_ENCRYPTION_KEY` — base64-encoded 32-byte key. Also honours the
   `_FILE` suffix (reads the key from a file path).
2. `${DATA_DIR}/shellyadmin.key` — generated on first boot if nothing else is
   configured. The file is written `0600`.

The key never leaves process memory at runtime; the SQLite file alone is not
enough to recover credentials. **Back up the key file alongside the database**
— losing the key permanently orphans every stored credential.

This defends against offline DB exposure (stolen backup, container escape
reading `/data`, misconfigured volume). It does not defend against an attacker
with live access to the running process.

## Network Scope

- scans may target any user-entered CIDR
- subnet-size and operational safeguards should still exist
- provisioning targets are validated as IP addresses and restricted to local/private/link-local ranges
- loopback, unspecified, and multicast targets are rejected for provisioning

## Operational Safety

- scan and refresh workflows use bounded worker pools
- concurrency limits cap active probes instead of spawning one goroutine per target

## Container Security

Recommended runtime posture:

- non-root user
- read-only root filesystem
- persistent writable data volume only where needed
- dropped Linux capabilities
- no-new-privileges

## TLS

The app itself does not need to terminate TLS.

Recommended pattern:

- optional reverse proxy terminates TLS
- app remains private and internal

## Explicit Constraints

The current design intentionally does not include:

- multi-user RBAC
- external API tokens
- public internet hardening beyond sensible defaults for a LAN appliance
