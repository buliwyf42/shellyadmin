# ShellyAdmin

ShellyAdmin is a self-hosted web app for discovering, inventorying, checking, and managing Shelly devices on a trusted local network.

It is designed as a single-container deployment with:

- staged device discovery before enrollment
- latest observed device state in SQLite
- manual firmware and provisioning workflows
- compliance checks against configured rules
- guided provisioning for normal use
- advanced provisioning mode for expert use
- separate audit and debug logging

## Status

This repository is under active development.

The target architecture is documented in [docs/ARCHITECTURE.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/ARCHITECTURE.md).

## Goals

- Easy to run in Docker
- Optimized for a single trusted operator on a LAN
- Supports both Gen1 and Gen2+ Shelly devices
- Keeps risky actions manual and previewed

## Current Feature Set

- Scan with staging and explicit add-to-inventory
- Device inventory table with sortable columns and per-user column visibility
- Auto-refresh in Devices view (30s, 1m, 5m)
- Per-device row actions in Devices view:
  - immediate refresh
  - delete/forget
- Compliance status in Devices view with hover details
- Gen-aware rendering for unsupported fields (for example WebSocket on Gen1)
- Manual firmware check and update flow
- Guided provisioning form plus JSON mode
- Provisioning target validation (local/private/link-local IPs only)
- Compliance rule editor (including `{device_name}` token matching)
- Separate audit events and debug logs views

## Planned / In Progress

- Export flows for devices, logs, templates, and jobs
- Stronger preview/validation flows for all risky actions
- Advanced-mode gating in UI settings
- Additional Docker network guidance surfaced in UI

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

The Vite dev server proxies `/api`, `/login`, `/logout`, and `/health` to the backend on `127.0.0.1:8080`.

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

See [docs/DEPLOYMENT.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/DEPLOYMENT.md) for deployment guidance.

## Security

This project is intended for trusted LAN use, not direct internet exposure.

See [docs/SECURITY.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/SECURITY.md) for the current security model and deployment expectations.

## Architecture

The current agreed architecture is documented in:

- [docs/ARCHITECTURE.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/ARCHITECTURE.md)
- [docs/adr/README.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/adr/README.md)

## Contributing

The project is still being shaped, so architecture changes should align with the documented design goals before implementation.
