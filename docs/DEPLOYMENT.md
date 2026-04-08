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

## Restore

Restore by replacing the contents of the data volume while the container is stopped.

## Non-Goals

This deployment model does not require:

- Postgres
- Redis
- a separate worker container
- a multi-service control plane
