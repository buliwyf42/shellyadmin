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
Current UI/API baseline is `v0.0.4`.

Public support posture:

- experimental but usable for trusted-LAN administration
- optimized for a single trusted operator
- not intended for direct internet exposure
- not yet positioned as a multi-user or HA-ready platform

Current `0.0.4` highlights:

- configurable device refresh timeout in Settings
- stale-device signaling in Devices when the latest refresh attempt fails
- `Last Success` wording in Devices to distinguish successful retrieval from the latest attempted refresh
- database migration support for refresh-state tracking
- GitHub Actions workflow for publishing a `ShellyAdmin` container image to GHCR
- Docker Compose aligned with the published `ghcr.io/buliwyf42/shellyadmin` image name

Current working tree highlights beyond `v0.0.4`:

- previewable bulk settings actions in Devices
- per-device detail page with raw config/status snapshots and safe manual actions
- locale-aware `Last Success` presentation shared between Devices and per-device detail
- documented `v1` API contract exposed at `/api/openapi/v1.json` and in the UI at `/docs`, covering the supported session, inventory, workflow, settings, backup, and audit routes
- optional mDNS-assisted discovery toggle in Settings

The target architecture is documented in [docs/ARCHITECTURE.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/ARCHITECTURE.md).

## Goals

- Easy to run in Docker
- Optimized for a single trusted operator on a LAN
- Supports both Gen1 and Gen2+ Shelly devices
- Keeps risky actions manual and previewed

## Quick Start

Fastest Docker run for a trusted LAN test setup:

```bash
docker run -d \
  --name shellyadmin \
  -p 8080:8080 \
  -v shellyadmin-data:/data \
  -e SHELLYADMIN_PASS='change-this-admin-password' \
  -e SHELLYADMIN_SECRET='change-this-cookie-secret' \
  -e COOKIE_SECURE=false \
  ghcr.io/buliwyf42/shellyadmin:latest
```

Then open `http://localhost:8080`.

For a Compose-based deployment, create the secret files expected by [`docker/docker-compose.yml`](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docker/docker-compose.yml) and run:

```bash
docker compose -f docker/docker-compose.yml up -d
```

Use strong secrets for real installs. The `COOKIE_SECURE=false` example above is only for plain HTTP on a trusted LAN.

## Current Feature Set

- Scan with staging and explicit add-to-inventory
- Device inventory table with sortable columns and per-user column visibility
- Bulk actions with preview/apply for timezone, MQTT, location, and 24-hour time settings
- Auto-refresh in Devices view (30s, 1m, 5m)
- Separate scan timeout and refresh timeout in Settings
- Optional mDNS-assisted discovery in addition to subnet scanning
- Per-device row actions in Devices view:
  - immediate refresh
  - delete/forget
- Per-device detail view with:
  - raw config/status snapshots
  - discovered capabilities
  - safe single-device actions (refresh, firmware check/update, reboot)
- Locale-aware relative/absolute time presentation in both Devices and per-device detail
- Stale row signal when the latest refresh attempt fails
- Compliance status in Devices view with hover details
- Gen-aware rendering for unsupported fields (for example WebSocket on Gen1)
- Manual firmware check and update flow
- Guided provisioning form plus JSON mode
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

- Export flows for devices, logs, templates, and jobs
- Advanced-mode gating in UI settings
- Broader action discovery for device components where protocol support is reliable
- CLI after the external API contract settles

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

- tagged source releases such as `v0.0.4`
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
