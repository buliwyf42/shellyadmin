# ShellyAdmin Architecture

See the agreed reference architecture in the external planning copy if needed, but this repository version is the source of truth for implementation.

Detailed accepted decisions are tracked in [docs/adr/README.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/adr/README.md).

## Summary

ShellyAdmin is a single-container, LAN-only, single-user operations console for Shelly devices.

Core principles:

- SQLite-backed
- staged discovery
- current observed state only
- manual, previewed operations
- guided provisioning first
- advanced provisioning available via JSON mode
- separate audit and debug logs
- advisory compliance model
- manual-first risky operations

## Decision Baseline

The current accepted decision set includes:

- [ADR-0001 Product Scope and Explicit Non-Goals](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/adr/0001-product-scope-and-non-goals.md)
- [ADR-0002 Operational Safety and Compliance Model](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/adr/0002-operational-safety-and-compliance.md)
- [ADR-0003 Device Authentication and Credentials Roadmap](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/adr/0003-device-auth-and-credentials.md)
- [ADR-0004 Job Concurrency and Execution Semantics](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/adr/0004-job-concurrency-and-execution.md)
- [ADR-0005 Data, Migration, and Compatibility Policy](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/adr/0005-data-migrations-and-compatibility.md)
- [ADR-0006 Backup, Export/Import, and Secret Handling](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/adr/0006-backup-export-import-and-secrets.md)
- [ADR-0007 UI Time and Error Presentation Policy](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/adr/0007-ui-time-and-error-presentation.md)

## Layers

- `internal/api`: HTTP endpoints and session handling
- `internal/services`: workflows and job orchestration
- `internal/core`: Shelly protocol logic
- `internal/db`: persistence and migrations
- `web`: embedded Svelte UI

## Data Model

Primary tables:

- `devices`
- `settings`
- `templates`
- `jobs`
- `audit_log`

No historical device-state timeline is required.

## Job Policy

Auto-restart after container restart:

- scan
- refresh
- firmware check

Do not auto-restart:

- firmware update
- provision

## Provisioning

Two modes:

1. Guided mode
2. Advanced mode

Current implementation exposes both modes directly in the Provision view.

Provisioning safety constraints:

- targets must be valid IP addresses
- only local/private/link-local targets are allowed
- loopback/unspecified/multicast targets are blocked

## Logging

- audit events in SQLite
- debug/probe traces in rotating file logs

## Runtime Concurrency

- scan and refresh jobs run through bounded worker pools
- configured concurrency limits active network probes and prevents unbounded goroutine fan-out

## Deployment

Primary target:

- one Docker container

Optional:

- reverse proxy with TLS termination

## UI Notes (Current)

- Devices is the primary operational surface:
  - sortable table
  - configurable columns
  - auto refresh
  - per-row refresh/delete actions
- Per-device compliance details are shown via the compliance badge hover in Devices.
