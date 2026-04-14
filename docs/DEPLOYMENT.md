# Deployment

## Overview

ShellyAdmin is designed to be attractive as a single-container deployment.

Primary target:

- one app container
- one SQLite database
- one persistent data volume

Supported environments:

- plain HTTP on a trusted LAN
- optional reverse proxy with TLS termination
- Docker or Compose driven deployment from a tagged GitHub checkout

## Environment Variables

Supported runtime variables:

- `SHELLYADMIN_USER`
- `SHELLYADMIN_PASS`
- `SHELLYADMIN_PASS_FILE`
- `SHELLYADMIN_SECRET`
- `SHELLYADMIN_SECRET_FILE`
- `DATA_DIR`
- `PORT`
- `COOKIE_SECURE`

Recommended:

- use `*_FILE` variants for container secrets
- set `COOKIE_SECURE=true` when behind TLS

## Docker Compose

The repo includes:

- [docker/Dockerfile](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docker/Dockerfile)
- [docker/docker-compose.yml](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docker/docker-compose.yml)

Current expected flow:

1. Check out a tagged GitHub release, for example `v0.0.3`
2. Provide the required secrets files
3. Build and run with Compose from the repository root

Example:

```bash
git clone https://github.com/buliwyf42/shellyadmin.git
cd shellyadmin
git checkout v0.0.3
docker compose -f docker/docker-compose.yml up -d --build
```

Notes:

- the current Compose file builds from local source instead of pulling a published image
- this keeps the embedded frontend bundle and backend binary aligned with the checked-out release tag
- a future GitHub container publishing workflow can layer on top of this without changing the runtime model

Recommended production characteristics:

- non-root container user
- read-only root filesystem
- persistent `/data`
- dropped Linux capabilities
- `tmpfs` for `/tmp`
- healthcheck enabled

## Networking

ShellyAdmin should work in both:

- bridge networking
- host networking

Important note:

- discovery behavior may differ depending on Docker networking
- host networking may be more reliable in some LAN environments
- longer device refresh timeouts may be useful when weaker Wi-Fi links delay refresh responses

The UI should eventually warn when Docker networking may limit discovery.

## Reverse Proxy

Reverse proxy support is optional but recommended for more serious installs.

Suggested pattern:

- reverse proxy handles TLS
- ShellyAdmin stays on plain HTTP inside the local environment
- `COOKIE_SECURE=true` when served through TLS

## Backups

Back up the persistent data volume, especially:

- `shellyctl.db`
- `shellyctl.log`

The SQLite database contains:

- inventory
- settings
- templates
- jobs
- audit events
- device refresh-state metadata used for stale/fresh signaling in the UI

## Restore

Restore by replacing the contents of the data volume while the container is stopped.

## Non-Goals

This deployment model does not require:

- Postgres
- Redis
- a separate worker container
- a multi-service control plane
