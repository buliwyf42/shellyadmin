# ShellyAdmin

ShellyAdmin is a self-hosted web app for discovering, inventorying, checking, and managing Shelly devices on a trusted local network.

It is designed as a single-container deployment with:

- staged device discovery before enrollment
- latest observed device state in SQLite
- manual firmware and provisioning workflows
- compliance checks against configured rules
- guided provisioning for normal use
- advanced provisioning mode for expert use
- audit logging in-app

## Status

This repository is under active development.
Current UI/API baseline is `v0.1.11`.

Public support posture:

- experimental but usable for trusted-LAN administration
- optimized for a single trusted operator
- not intended for direct internet exposure
- not yet positioned as a multi-user or HA-ready platform

The target architecture is documented in [docs/ARCHITECTURE.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/ARCHITECTURE.md).

## Goals

- Easy to run in Docker
- Optimized for a single trusted operator on a LAN
- Supports Gen2+ Shelly devices
- Keeps risky actions manual and previewed

## Quick Start

Fastest Docker run for a trusted LAN test setup. Prefer
`SHELLYADMIN_PASS_HASH` (argon2id) — generate one with
`docker run --rm ghcr.io/buliwyf42/shellyadmin:latest shellyctl hash-password <plaintext>`:

```bash
docker run -d \
  --name shellyadmin \
  -p 8080:8080 \
  -v shellyadmin-data:/data \
  -e SHELLYADMIN_PASS_HASH='$argon2id$v=19$m=65536,t=2,p=1$…' \
  -e SHELLYADMIN_SECRET='change-this-cookie-secret' \
  -e COOKIE_SECURE=false \
  ghcr.io/buliwyf42/shellyadmin:latest
```

`SHELLYADMIN_PASS` (plaintext) still works for backward compatibility but logs
a deprecation warning on startup.

Then open `http://localhost:8080`.

For a Compose-based deployment, create the secret files expected by [`docker/docker-compose.yml`](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docker/docker-compose.yml) and run:

```bash
docker compose -f docker/docker-compose.yml up -d
```

Use strong secrets for real installs. The `COOKIE_SECURE=false` example above is only for plain HTTP on a trusted LAN.

## Current Feature Set

- Scan with staging and explicit add-to-inventory
- Device inventory table with sortable columns and per-user column visibility
- Bulk actions with preview/apply for timezone, MQTT, location, SNTP, and reboot
- Auto-refresh in Devices view (30s, 1m, 5m)
- Separate scan timeout and refresh timeout in Settings
- Optional mDNS-assisted discovery in addition to subnet scanning
- Per-device row actions in Devices view:
  - immediate refresh
  - per-row reboot (⏻) with inline spinner
  - delete/forget
- Reboot All toolbar button for bulk device reboots
- Per-device detail view with:
  - raw config/status snapshots
  - discovered capabilities
  - safe single-device actions (refresh, firmware check/update, reboot)
- Locale-aware relative/absolute time presentation in both Devices and per-device detail
- Stale row signal when the latest refresh attempt fails
- Compliance status in Devices view with hover details
- Manual firmware check and update flow:
  - per-device, per-channel availability cache (stable + beta read in a single check)
  - sortable, select-all-aware Firmware page with a confirmation modal before bulk install
  - dedicated install job with per-device version-match polling and a 5-minute timeout
  - bulk auto-update controls (Off / Stable / Beta) implemented via `Schedule.*`, the same mechanism the device's own web UI uses
  - shared Stable/Beta channel between the Firmware and Devices pages (persisted to localStorage)
- Guided provisioning form plus JSON mode with template management (load, save, delete, rename) in-context:
  - full `Wifi.SetConfig` surface: primary STA, secondary STA (STA1), roaming (RSSI threshold, interval), static IPv4 per STA
  - Script section (per-id loop), UI.SetConfig, Ethernet IPv6/DNS
  - `auto_update` section (off / stable / beta) — synthesised onto the device as a `Schedule.*` job
  - `restart_required` badge per device in results; "Reboot restart-required devices" button
- Auth Groups page:
  - groups contain their own auth credentials (`username`, `password`/`ha1`, tags)
  - device-to-group assignment for future auth-required workflows
- Provisioning target validation (local/private/link-local IPs only)
- Compliance rule editor (including `{device_name}` token matching)
- Backup/export/import with dry-run and apply:
  - settings
  - templates
  - auth groups
  - device-group assignments
- Audit logs view (debug log mode removed)
- Documented API surface and OpenAPI JSON for the supported v1 routes

## Planned / In Progress

See [docs/roadmap.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/roadmap.md) for the current roadmap. Headline items:

- Broader action discovery for device components where protocol support is reliable
- CLI after the external API contract settles
- API stability guarantee (v0.x may have breaking changes; semver will apply from v1.0.0)

## Project Structure

```text
cmd/shellyctl        Application entrypoint
internal/api         HTTP routing and handlers
internal/services    Workflow orchestration
internal/core        Shelly protocol logic
internal/db          SQLite persistence and migrations
internal/models      Shared data models
web                  Svelte frontend
docker               Container files
docs                 Product and deployment documentation
```

## Running Locally

### Backend

Requirements:

- Go 1.24+

Run:

```bash
make dev-backend
```

### Frontend

Requirements:

- Node 20+

Run:

```bash
cd web
npm install
npm run dev
```

The Vite dev server proxies `/api` and `/health` to the backend on `127.0.0.1:8080`.

Recommended native workflow:

1. Run the backend:

```bash
make dev-backend
```

2. In a second terminal, run the frontend:

```bash
cd web
npm install
npm run dev
```

3. Open the app through the Vite dev server on `http://<host>:5173`

If you want to run the Go app with embedded static assets instead, build and sync the frontend first:

```bash
make frontend
make backend
./bin/shellyctl
```

## Docker

The app is intended to run as a single container.

For GitHub-based distribution, the repository should be treated as the source of truth for:

- tagged source releases such as `v0.0.6`
- the Docker build context under [`docker/`](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docker)
- the GitHub Actions flow that publishes a versioned image to GHCR per release tag

The included Compose file is aligned with the published GHCR image name:

- default image: `ghcr.io/buliwyf42/shellyadmin:latest`
- optional local rebuild path: `docker compose -f docker/docker-compose.yml up -d --build`

Compatibility note:

- internal binary and SQLite filenames still use `shellyctl` for now
- the external product, Docker, and GitHub-facing name remains `ShellyAdmin`

See [docs/DEPLOYMENT.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/DEPLOYMENT.md) for deployment guidance.

## What Works Today

- device discovery with explicit inventory enrollment
- periodic and manual device refresh
- compliance rule management and device compliance visibility
- firmware inspection and update flow
- guided provisioning plus advanced JSON mode
- auth groups, backup export/import, and audit logs

## Not Production-Grade Yet

- direct internet exposure
- multi-user RBAC
- high availability deployments
- public API lifecycle guarantees
- broad automated recovery and self-healing flows

## Security

This project is intended for trusted LAN use, not direct internet exposure.

See [SECURITY.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/SECURITY.md) and [docs/SECURITY.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/SECURITY.md) for the current security model and deployment expectations.

## Architecture

The current agreed architecture is documented in:

- [docs/ARCHITECTURE.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/ARCHITECTURE.md)
- [docs/adr/README.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/adr/README.md)

## Contributing

The project is still being shaped, so architecture changes should align with the documented design goals before implementation.

See [CONTRIBUTING.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/CONTRIBUTING.md) for the development and PR workflow.
