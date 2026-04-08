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

## Planned Feature Set

- Network scan and staging area for discovered devices
- Device inventory and refresh
- Compliance checking
- Manual firmware check and update
- Guided provisioning templates
- Advanced raw provisioning mode
- Export support for devices, logs, templates, and jobs

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

- Go 1.23+

Run:

```bash
SHELLYADMIN_PASS=dev-secret go run ./cmd/shellyctl
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

## Docker

The app is intended to run as a single container.

See [docs/DEPLOYMENT.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/DEPLOYMENT.md) for deployment guidance.

## Security

This project is intended for trusted LAN use, not direct internet exposure.

See [docs/SECURITY.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/SECURITY.md) for the current security model and deployment expectations.

## Architecture

The current agreed architecture is documented in:

- [docs/ARCHITECTURE.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/ARCHITECTURE.md)

## Contributing

The project is still being shaped, so architecture changes should align with the documented design goals before implementation.
