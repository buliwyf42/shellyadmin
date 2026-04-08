# ShellyAdmin Architecture

See the agreed reference architecture in the external planning copy if needed, but this repository version is the source of truth for implementation.

## Summary

ShellyAdmin is a single-container, LAN-only, single-user operations console for Shelly devices.

Core principles:

- SQLite-backed
- staged discovery
- current observed state only
- manual, previewed operations
- guided provisioning first
- advanced provisioning behind an explicit enablement step
- separate audit and debug logs

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

Advanced mode must be opt-in and clearly marked.

## Logging

- audit events in SQLite
- debug/probe traces in rotating file logs

## Deployment

Primary target:

- one Docker container

Optional:

- reverse proxy with TLS termination
