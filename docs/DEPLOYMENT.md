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
- Docker deployment from the published GHCR image

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

Current expected flows:

Published image:

```bash
docker run -d \
  --name shellyadmin \
  -p 8080:8080 \
  -v shellyadmin-data:/data \
  -e SHELLYADMIN_PASS=change-me \
  -e SHELLYADMIN_SECRET=change-me-too \
  -e COOKIE_SECURE=false \
  ghcr.io/buliwyf42/shellyadmin:latest
```

Tagged source checkout:

1. Check out a tagged GitHub release, for example `v0.0.4`
2. Provide the required secrets files
3. Build and run with Compose from the repository root

Example:

```bash
git clone https://github.com/buliwyf42/shellyadmin.git
cd shellyadmin
git checkout v0.0.4
docker compose -f docker/docker-compose.yml up -d --build
```

Notes:

- the Compose file uses `ghcr.io/buliwyf42/shellyadmin:latest` as its default image name
- `docker compose up -d` can use the published image directly
- `docker compose up -d --build` rebuilds locally from the checked-out source when you want an exact local release build
- GitHub Actions publishes versioned images for tags such as `v0.0.4`

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

Compatibility note:

- internal runtime filenames still use `shellyctl.db` and `shellyctl.log`
- those names are kept for now to avoid unnecessary migration churn

## Restore

Restore by replacing the contents of the data volume while the container is stopped.

## Non-Goals

This deployment model does not require:

- Postgres
- Redis
- a separate worker container
- a multi-service control plane
