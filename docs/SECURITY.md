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

Two log classes are intended:

1. Audit events
   - stored in SQLite
   - operator-facing
   - meaningful actions only

2. Debug logs
   - stored in rotating file logs
   - lower-level diagnostic traces
   - not mixed into audit history

Secrets should never be written to logs in raw form.

## Provisioning Secrets

Current product decision:

- templates may contain real secrets

Required safeguards:

- secret-bearing templates should be clearly marked in the UI
- exports should redact secret values by default
- password-derived device auth material should remain ephemeral during execution

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
