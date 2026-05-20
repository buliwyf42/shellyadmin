# ShellyAdmin Architecture

See the agreed reference architecture in the external planning copy if needed, but this repository version is the source of truth for implementation.

Detailed accepted decisions are tracked in [docs/adr/README.md](docs/adr/README.md).

## Summary

ShellyAdmin is a single-container, LAN-only, single-user operations console for Shelly devices.

Core principles:

- SQLite-backed
- staged discovery
- current observed state only
- manual, previewed operations
- guided provisioning first
- advanced provisioning available via JSON mode
- audit logs as the operator-facing log surface
- advisory compliance model
- manual-first risky operations

## Decision Baseline

The current accepted decision set includes:

- [ADR-0001 Product Scope and Explicit Non-Goals](docs/adr/0001-product-scope-and-non-goals.md)
- [ADR-0002 Operational Safety and Compliance Model](docs/adr/0002-operational-safety-and-compliance.md)
- [ADR-0003 Device Authentication and Credentials Roadmap](docs/adr/0003-device-auth-and-credentials.md)
- [ADR-0004 Job Concurrency and Execution Semantics](docs/adr/0004-job-concurrency-and-execution.md)
- [ADR-0005 Data, Migration, and Compatibility Policy](docs/adr/0005-data-migrations-and-compatibility.md)
- [ADR-0006 Backup, Export/Import, and Secret Handling](docs/adr/0006-backup-export-import-and-secrets.md)
- [ADR-0007 UI Time and Error Presentation Policy](docs/adr/0007-ui-time-and-error-presentation.md)
- [ADR-0008 Provision/Compliance UI Alignment and Template Management Consolidation](docs/adr/0008-provision-compliance-ui-alignment.md)
- [ADR-0009 Firmware Auto-Update via `Schedule.*` Synthesis](docs/adr/0009-firmware-auto-update-via-schedule.md)
- [ADR-0010 Per-Device Action Discovery via `Shelly.ListMethods`](docs/adr/0010-per-device-action-discovery-via-listmethods.md)

## Layers

- `internal/api`: HTTP endpoints and session handling
- `internal/services`: workflows and job orchestration, split by topic:
  - `app.go` — service struct, lifecycle, settings, templates, logs, shared helpers
  - `app_jobs.go` — refresh / scan / firmware-check / firmware-install job orchestration and lifecycle
  - `app_clients.go` — per-device option builders (scanner / setter / firmware / provisioner) and the cross-cutting `refreshFirmwareCache` helper
  - `app_backup.go` — backup export/import and dry-run impact reporting
  - `app_credentials.go` — credential and credential-group CRUD
- `internal/core`: Shelly protocol logic — `scanner`, `firmware`, `setters`, `provisioner`, `compliance`, `shellyclient`, `secretbox`, plus the small `clock` package that abstracts wall time so the protocol packages can be tested deterministically without mocking the time package globally. `internal/core/firmware/autoupdate.go` reads/writes the per-device auto-update mode by manipulating `Schedule.*` jobs (Shelly Gen2+ has no dedicated OTA-config method)
- `internal/mcp`: optional read-only Model Context Protocol server (added v0.1.19). When `SHELLYADMIN_MCP_TOKEN` is set, the binary starts a second HTTP listener on `:8081` exposing 13 read-only tools as thin adapters over `services.AppService`. Listener is off by default; bearer-token gated. Design rationale in [adr/0011-mcp-read-only-server.md](./adr/0011-mcp-read-only-server.md).
- `internal/db`: persistence and migrations
- `web`: embedded Svelte UI

### Testability seams (M3, v0.1.15)

Each device-talking package exposes its public entry point in two layers:

- `…WithOptions(ctx, …, opts Options)` — production callers. Builds a `*shellyclient.Client` from `Options`, then delegates.
- `…OnClient(ctx, client, …)` — accepts a pre-built `shellyclient.Client` directly. Tests construct an `httptest.NewServer` fake-Shelly + a client aimed at it.

`scanner.ProbeOptions` and `firmware.Options` carry an optional `Clock clock.Clock` field — nil falls back to `clock.Real()`. Tests inject `clock.NewFake(t)` + `Advance(d)` to pin timestamp-bearing fields to deterministic values. Production behaviour is unchanged from before the seam was added.

## Data Model

Primary tables:

- `devices`
- `settings`
- `templates`
- `credentials`
- `credential_groups`
- `device_credential_groups`
- `jobs`
- `audit_log`

No historical device-state timeline is required.

Current device-state nuance:

- `last_seen` represents the last successful device retrieval
- the latest refresh attempt status is tracked separately so the UI can mark a device as stale even when an older successful snapshot exists

## Job Policy

Auto-restart after container restart:

- scan
- refresh
- firmware check (re-issues `Shelly.CheckForUpdate` + `Schedule.List` per device)

Do not auto-restart:

- firmware install (`firmware_install` job — bulk `Shelly.Update` + per-device version polling)
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
- UI exposes audit logs only

## Runtime Concurrency

- scan and refresh jobs run through bounded worker pools
- configured concurrency limits active network probes and prevents unbounded goroutine fan-out
- `AppService` owns a root context; on shutdown (SIGTERM) it is cancelled, in-flight jobs observe the cancellation, and their job rows are marked `interrupted` immediately rather than waiting for the stale-job guard

## API Versioning Policy

Pre-v1.0 release lines (current: 0.x) ship without API version
guarantees. The HTTP API surface is documented under the
`info.version: "v1"` field in the OpenAPI spec served at
`/api/openapi/v1.json`, but breaking changes between minor releases
are reserved until the explicit pre-1.0 line clears.

When v1.0 lands, the policy hardens to:

- **`/api/v1/*`** stays stable. Routes can gain new optional query
  parameters and new optional response fields; existing required
  fields cannot rename or change shape.
- **Breaking changes** ship behind a new `/api/v2/` prefix. The
  v1 routes continue to work for one full release cycle, marked
  with a `Deprecation: true` response header so client tooling
  (the SPA, MCP clients, ad-hoc scripts) can warn before the
  removal window closes.
- **Removal**: a `/api/v1/*` route may be deleted at the v3 cut,
  not before. Operators get two release lines to migrate their
  external scripts.

The OpenAPI document's `paths` keyspace is the contract surface.
Adding a new route is non-breaking. Renaming a request/response
field is breaking. Adding a new required field is breaking. Adding
a new optional field is non-breaking. The `cmd/modelschema` drift
check (Phase 3 / M3) catches accidental Go-struct changes that
would tilt the wire shape.

This policy is the T7 from the consolidated review's Phase 4
shortlist. Concrete `/api/v1/*` prefix mounting + the `Deprecation`
header are queued for the v0.3 → v1.0 cut; the policy itself is
codified here so the next person to ship a breaking change has a
written rule to follow.

## Deployment

Primary target:

- one Docker container

Current distribution assumption:

- GitHub tagged source releases are the canonical release artifact
- container builds are expected to be reproducible directly from the tagged repository checkout
- GHCR-published images are the canonical container artifact for release consumption

Optional:

- reverse proxy with TLS termination

## UI Notes (Current)

- Devices is the primary operational surface:
  - sortable table
  - configurable columns
  - auto refresh
  - configurable refresh timeout
  - per-row refresh/delete actions
  - stale/fresh signal based on latest refresh outcome
- Time presentation follows ADR-0007:
  - locale-aware display in Devices
  - the per-device detail page reuses the same `Last Success` formatting policy
- Per-device compliance details are shown via the compliance badge hover in Devices.
- Per-device detail is the secondary operational surface:
  - status summary
  - capabilities
  - safe single-device actions
  - raw config/status snapshots for troubleshooting
- Provision, Auth Groups, and Compliance follow a shared two-column layout:
  - left: settings/rules/groups
  - right: device list
- Template management lives entirely on the Provision page (see ADR-0008):
  - load, edit, save, delete, and rename templates in-context
  - Settings no longer exposes a Templates section
- Provision and Compliance share a common section and field ordering policy (see ADR-0008):
  - section order: sys → mqtt → cloud → ws → ble → wifi → auto_update → matter → modbus → zigbee
  - sys field order: name (provision only) → tz → sntp → debug_ws → debug_udp → rpc_udp → lat → lon → eco → discoverable
  - the legacy `ota` section is no longer rendered; auto-update lives under its own `auto_update` section, implemented via `Schedule.*` rather than a (non-existent) `OTA.SetConfig` method
- Firmware page shares the Stable/Beta channel selector with the Devices page via a localStorage-backed Svelte store (`firmwareChannel`). Toggling on either page propagates immediately.
- Generation badge colors (Gen 2 / 3 / 4) are operator-configurable via Settings → UI Preferences and shared between the Devices and Firmware pages.
