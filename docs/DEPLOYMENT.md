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

| Variable                               | Purpose                                                                        | Notes                                                                                                                                                                  |
| -------------------------------------- | ------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `SHELLYADMIN_USER`                     | Admin username                                                                 | Default `admin`. Only used as the default username for the one-time env import below; once set up, the username lives in the DB.                                       |
| `SHELLYADMIN_PASS_HASH` / `_FILE`      | _Optional_ — argon2id PHC string from `shellyctl hash-password <plaintext>`    | No longer required (ADR-0017). With no credential configured the server boots into **first-run setup** and you create the admin account in the web UI. If this var is set on a fresh instance, the hash is imported into the DB **once** at boot (seamless upgrade), after which it is irrelevant. Recover a forgotten password with `shellyctl reset-auth --force`. |
| `SHELLYADMIN_SECRET` / `_FILE`         | Cookie/session signing secret                                                  | Auto-generated if unset, but persists only for the process lifetime                                                                                                    |
| `SHELLYADMIN_ENCRYPTION_KEY` / `_FILE` | base64-encoded 32-byte key for credential at-rest encryption                   | **Required** since v0.3.0 (ADR-0013) — the binary refuses to start without it. Manage it externally (Docker secret, sops, NixOS secret); prefer the `_FILE` form. Keep it stable across recreates — losing or rotating it orphans every stored credential. |
| `DATA_DIR`                             | SQLite + key + log directory                                                   | Default `./data`                                                                                                                                                       |
| `PORT`                                 | HTTP listen port                                                               | Default `8080`                                                                                                                                                         |
| `COOKIE_SECURE`                        | `true` to send the `Secure` flag on session cookies                            | Set when behind TLS                                                                                                                                                    |

Recommended:

- use the `_HASH_FILE` indirection in containers so the hash itself doesn't sit in environment files or container manifests
- set `COOKIE_SECURE=true` when behind TLS

### Configurable knobs surfaced in Settings

These live in the SQLite `settings` row, not env vars, but are worth knowing during deployment:

- **Firmware install timeout** (`firmware_install_timeout`, default `300` s): per-device cap before the install_job marks "unknown".
- **Firmware install poll cadence** (`firmware_install_poll_interval`, default `5` s, bounded `[1, 60]`): how often the install_job re-queries each device's firmware version while waiting for the post-`Shelly.Update` reboot. Lower for snappier feedback on a small fleet, raise for slow devices.
- **Scheduled firmware check** (`firmware_check_interval`, default `0` = off): periodic fleet-wide `firmware_check` cadence in seconds.

## Docker Compose

The repo includes:

- [docker/Dockerfile](docker/Dockerfile)
- [docker/docker-compose.yml](docker/docker-compose.yml)

Current expected flows:

Published image — generate the hash and a stable encryption key, then run:

```bash
HASH=$(docker run --rm ghcr.io/buliwyf42/shellyadmin:latest hash-password 'change-this-admin-password')
# Required since v0.3.0. SAVE THIS — reuse the same key on every recreate,
# or you orphan all stored credentials.
KEY=$(openssl rand -base64 32)
docker run -d \
  --name shellyadmin \
  -p 8080:8080 \
  -v shellyadmin-data:/data \
  -e SHELLYADMIN_PASS_HASH="$HASH" \
  -e SHELLYADMIN_SECRET='change-this-cookie-secret' \
  -e SHELLYADMIN_ENCRYPTION_KEY="$KEY" \
  -e COOKIE_SECURE=false \
  ghcr.io/buliwyf42/shellyadmin:latest
```

Add `-p 8081:8081` (or remap to a different host port, e.g. `-p 8101:8081`) if you plan to enable the **MCP listener** via the Settings UI or `SHELLYADMIN_MCP_TOKEN` env var. The container exposes `:8081` for MCP but it's only bound when a token is configured — see ADR-0011 for the read+write tool surface and the v0.2.3 stdio alternative (`shellyctl mcp` subcommand for Claude Desktop on the same host).

This quick-start example is only for plain HTTP on a trusted LAN. For a more durable deployment, use the `*_FILE` secret variants and set `COOKIE_SECURE=true` when serving through TLS.

Tagged source checkout:

1. Check out a tagged GitHub release (e.g. `v0.1.18`).
2. Provide the required secrets files (the bundled compose expects them under `secrets/`).
3. Build and run with Compose from the repository root.

Example:

```bash
git clone https://github.com/buliwyf42/shellyadmin.git
cd shellyadmin
git checkout v0.1.18
mkdir -p secrets
# Admin password: write the argon2id HASH (not a plaintext or random value —
# the app validates it as an argon2id PHC string). SHELLYADMIN_PASS_HASH_FILE
# is the compose default.
docker run --rm ghcr.io/buliwyf42/shellyadmin:latest hash-password 'change-this-admin-password' > secrets/shellyadmin_pass.txt
# Cookie/session signing secret.
openssl rand -base64 32 > secrets/shellyadmin_secret.txt
# Encryption key — REQUIRED since v0.3.0 (base64 of 32 bytes). Keep it stable.
openssl rand -base64 32 > secrets/shellyadmin_encryption_key.txt
docker compose -f docker/docker-compose.yml up -d --build
```

Notes:

- the Compose file uses `ghcr.io/buliwyf42/shellyadmin:latest` as its default image name
- `docker compose up -d` can use the published image directly
- `docker compose up -d --build` rebuilds locally from the checked-out source when you want an exact local release build
- GitHub Actions publishes versioned images for tags such as `v0.1.6`

Recommended production characteristics:

- non-root container user
- read-only root filesystem
- persistent `/data`
- dropped Linux capabilities
- `tmpfs` for `/tmp`
- healthcheck enabled

Public-readiness note:

- the app is intended for trusted LAN use only
- do not expose it directly to the public internet
- validate discovery behavior in your Docker networking mode before depending on it operationally

### Verifying image signatures (cosign, v0.2.10+)

The `publish-image.yml` workflow keyless-signs every pushed image
through Sigstore using the GitHub OIDC issuer. Before pulling a new
release on a production host, verify the signature:

```bash
cosign verify \
  --certificate-identity-regexp 'https://github.com/buliwyf42/shellyadmin/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/buliwyf42/shellyadmin:vX.Y.Z
```

`cosign verify` exits non-zero if the image was not signed by this
repository's workflow, which catches a stolen GHCR write token or a
registry-side pivot. The signing key itself is ephemeral (Sigstore
Fulcio issues a short-lived cert per run) so there is no long-term
key for an attacker to steal.

For automated deploys, gate `docker pull` behind a successful
`cosign verify` in the deploy script — without it, the signing step
in CI is just bookkeeping.

The same workflow runs Trivy against the freshly-pushed image. A
HIGH/CRITICAL CVE fails the workflow after the push, so the tag
exists but you receive a notification before rolling production
forward. Re-pull only after the CI job completes.

## Networking

ShellyAdmin should work in both:

- bridge networking
- host networking

Important note:

- discovery behavior may differ depending on Docker networking
- host networking may be more reliable in some LAN environments
- mDNS-assisted discovery usually needs multicast visibility and therefore benefits from host networking on Linux Docker hosts
- longer device refresh timeouts may be useful when weaker Wi-Fi links delay refresh responses
- if mDNS discovery appears empty while subnet scanning still works, bridge networking is the first thing to check

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

### Pre-deploy snapshot

Before recreating the stack on a release, snapshot the SQLite file as a
rollback point. The database lives on the host bind mount, and the
container runs with a read-only root filesystem, so the copy must run on
the **host** (not via a container exec). Use the helper:

```bash
scripts/snapshot-prod-db.sh docker.home.lan v0.3.6
# -> /docker/shellyadmin/shellyctl.db.pre-v0.3.6-<epoch>
```

or run the equivalent directly on the host:

```bash
cp /docker/shellyadmin/shellyctl.db \
   /docker/shellyadmin/shellyctl.db.pre-v0.3.6-$(date +%s)
```

These accumulate by design as rollback points. The snapshot mainly guards
releases that carry a DB migration; a pure frontend/CI release does not
touch the schema, so rollback there is just redeploying the previous image
against the same database.

## Restore

Restore by replacing the contents of the data volume while the container is stopped.

## Standalone Binary Distribution (planned)

The canonical artifact is the GHCR container image. Operators who
prefer to run the Go binary directly (Linux service, NixOS module,
homelab CI runner) currently build from source — `go build
./cmd/shellyctl` — which produces an unsigned, unverified binary.

Phase 4 / T12 from the consolidated review queues the following for
a future release:

1. **goreleaser config** (`.goreleaser.yaml`) wiring multi-arch
   binary builds + checksum file generation.
2. **cosign blob signing** of every binary published to a GitHub
   Release. The same keyless-OIDC pattern v0.2.11+ uses for the
   container image (see `publish-image.yml`); operators get a
   `.sig` file alongside each binary they can verify with:
   ```
   cosign verify-blob \
     --certificate-identity-regexp 'https://github.com/buliwyf42/shellyadmin/' \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com \
     --signature shellyctl-linux-amd64.sig \
     shellyctl-linux-amd64
   ```
3. **Provenance attestation** via SLSA Level 3 builders so the
   binary's build origin is independently verifiable.

Today operators wanting binaries should build locally and accept
that the resulting artifact is unsigned. The Docker image is the
verified path until T12 ships.

## Non-Goals

This deployment model does not require:

- Postgres
- Redis
- a separate worker container
- a multi-service control plane
