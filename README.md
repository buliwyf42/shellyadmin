# ShellyAdmin

**English** | [Deutsch](README.de.md)

[![CI](https://github.com/buliwyf42/shellyadmin/actions/workflows/test.yml/badge.svg)](https://github.com/buliwyf42/shellyadmin/actions/workflows/test.yml)
[![License: MIT](https://img.shields.io/github/license/buliwyf42/shellyadmin)](LICENSE)
[![Latest release](https://img.shields.io/github/v/release/buliwyf42/shellyadmin?sort=semver)](https://github.com/buliwyf42/shellyadmin/releases)
[![GHCR](https://img.shields.io/badge/ghcr.io-buliwyf42%2Fshellyadmin-blue?logo=docker)](https://github.com/buliwyf42/shellyadmin/pkgs/container/shellyadmin)
[![Go Report Card](https://goreportcard.com/badge/github.com/buliwyf42/shellyadmin)](https://goreportcard.com/report/github.com/buliwyf42/shellyadmin)

ShellyAdmin is a self-hosted web app for discovering, inventorying, checking, and managing Shelly Gen2+ devices on a trusted local network.

## Why ShellyAdmin?

The Shelly cloud requires opting each device into a third-party service. Home Assistant's Shelly integration covers control, automation, and a per-entity view but not fleet-wide firmware management, compliance auditing, bulk provisioning, or an audit log of operator actions. ShellyAdmin sits next to those tools as the **fleet ops console**: scan a subnet, enroll devices into an inventory, push templated config to many at once, check/install firmware in bulk, and verify each device matches a compliance rule set — with every action audited.

It is designed as a single-container deployment with:

- staged device discovery before enrollment
- latest observed device state in SQLite
- manual firmware and provisioning workflows
- compliance checks against configured rules
- guided provisioning for normal use
- advanced provisioning mode for expert use
- audit logging in-app

![ShellyAdmin Devices view — fleet inventory with online state, WiFi/MQTT/cloud status, firmware version, compliance, and per-row actions](docs/screenshots/devices.png)

<details>
<summary>More screenshots: Scan, Firmware, Provision, Compliance</summary>

### Scan — discovery workflow

![Discovery workflow: subnet scan progress and known-vs-new device split](docs/screenshots/scan.png)

### Firmware — fleet-wide check and install

![Firmware page: per-device current version, available stable/beta channels, auto-update mode, and bulk actions](docs/screenshots/firmware.png)

### Provision — templated config push

![Provision page: section-by-section template editor on the left, device multi-select on the right](docs/screenshots/provision.png)

### Compliance — rule editor and per-device status

![Compliance page: rule editor on the left, per-device compliant/non-compliant summary on the right](docs/screenshots/compliance.png)

</details>

## Status

Under active development. Current release is `v0.5.3` (hardening: `shellyctl rotate-key`, device-response limits + JSON-RPC envelope validation, template-section validation, CI race detector + frontend coverage gate); the UI/API baseline is unchanged since `v0.4.0`. The project follows pre-1.0 semver: minor versions may carry breaking changes. Semver guarantees apply from `v1.0.0`.

Intended posture:

- experimental but usable for trusted-LAN administration
- optimized for a single trusted operator
- not intended for direct internet exposure
- not yet positioned as a multi-user or HA-ready platform

The target architecture is documented in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Goals

- Easy to run in Docker
- Optimized for a single trusted operator on a LAN
- Supports Gen2+ Shelly devices (Gen1 is intentionally unsupported)
- Keeps risky actions manual and previewed

## Quick Start

Fastest Docker run for a trusted LAN test setup:

```bash
docker run -d \
  --name shellyadmin \
  -p 8080:8080 \
  -v shellyadmin-data:/data \
  -e SHELLYADMIN_SECRET="$(openssl rand -hex 32)" \
  -e SHELLYADMIN_ENCRYPTION_KEY="$(openssl rand -base64 32)" \
  -e COOKIE_SECURE=false \
  ghcr.io/buliwyf42/shellyadmin:latest
```

Then open `http://localhost:8080` and create the admin account on the **first-run setup** screen. Forgot the password later? `docker exec shellyadmin shellyctl reset-auth --force` returns the instance to setup mode.

`SHELLYADMIN_ENCRYPTION_KEY` is **required** since v0.3.0 — the container won't start without it. Generate it once and **reuse the same value** on every recreate; a new key orphans all stored credentials.

`COOKIE_SECURE=false` is only safe on plain HTTP over a trusted LAN. Set `COOKIE_SECURE=true` (and front the container with TLS) for any other deployment.

### Pre-seeding the login (optional)

To skip the setup screen on a fresh instance, generate a hash and pass it as `SHELLYADMIN_PASS_HASH`:

```bash
HASH="$(docker run --rm ghcr.io/buliwyf42/shellyadmin:latest hash-password 'change-this-admin-password')"

docker run -d \
  --name shellyadmin \
  -p 8080:8080 \
  -v shellyadmin-data:/data \
  -e SHELLYADMIN_SECRET="$(openssl rand -hex 32)" \
  -e SHELLYADMIN_ENCRYPTION_KEY="$(openssl rand -base64 32)" \
  -e SHELLYADMIN_PASS_HASH="$HASH" \
  -e COOKIE_SECURE=false \
  ghcr.io/buliwyf42/shellyadmin:latest
```

The hash is imported into the database once at boot, then ignored. To rotate the password after that, use Settings → Operator Account in the UI.

### Docker Compose

For a Compose-based deployment, see [`docker/docker-compose.yml`](docker/docker-compose.yml):

```bash
docker compose -f docker/docker-compose.yml up -d
```

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for the full deployment guide, including hardening flags, MCP exposure, and pre-deploy DB snapshots.

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
  - dedicated install job with per-device version-match polling; both the timeout (default 300 s) and the poll cadence (default 5 s, bounded 1–60) are operator-configurable in Settings
  - bulk auto-update controls (Off / Stable / Beta) implemented via `Schedule.*`, the same mechanism the device's own web UI uses
  - shared Stable/Beta channel between the Firmware and Devices pages (persisted to localStorage)
- Guided provisioning form plus JSON mode with template management (load, save, delete, rename) in-context:
  - full `Wifi.SetConfig` surface: primary STA, secondary STA (STA1), roaming (RSSI threshold, interval), static IPv4 per STA
  - Script section (per-id loop), UI.SetConfig, Ethernet IPv6/DNS
  - `auto_update` section (off / stable / beta) — synthesised onto the device as a `Schedule.*` job
  - **Webhooks** form (v0.2.4): `delete_all` toggle, delete-by-id, new-webhook entries (cid/event/name/enable/URLs)
  - **Cover** form (v0.2.5): id, name, maxtime open/close, swap_inputs, power_limit, and the FW 2.0.0-beta1 `slat` sub-object for venetian-blind tilt
  - **Zigbee operations** form (v0.2.6): write-mostly cards for `Zigbee.SendCommand` / `Zigbee.ReadAttr` / `Zigbee.WriteAttr`, generates a `gen2_rpc` template section
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
- Optional read-only [MCP server](#optional-mcp-server-read-only) for LLM-driven introspection

## Roadmap

See [docs/roadmap.md](docs/roadmap.md) for the current roadmap. Headline items:

- Broader action discovery for device components where protocol support is reliable
- `shellyctl` write commands (read-only CLI shipped in v0.3.6)
- API stability guarantee from `v1.0.0`

## Project Structure

```text
cmd/shellyctl        Application entrypoint (server + CLI subcommands)
internal/api         HTTP routing and handlers
internal/services    Workflow orchestration
internal/core        Shelly protocol logic (scanner, firmware, provisioner)
internal/mcp         Read-only MCP server (HTTP + stdio transports)
internal/db          SQLite persistence and migrations
internal/models      Shared data models
web                  Svelte frontend
docker               Container files
docs                 Architecture, deployment, and ADR documentation
```

## Running Locally

Requirements:

- Go 1.25+ (the `go.mod` floor; CI and the Docker build use the Go 1.26 toolchain)
- Node 22+ (CI and the Docker build use Node 26)

In two terminals:

```bash
# Terminal 1 — backend on :8080
make dev-backend

# Terminal 2 — frontend dev server on :5173 (proxies /api and /health)
cd web
npm install
npm run dev
```

Open `http://localhost:5173` and log in with `admin` / `dev-secret`.

To run the Go binary with embedded frontend assets instead of the Vite dev server:

```bash
make frontend   # build + sync the SPA into cmd/shellyctl/dist
make backend
./bin/shellyctl
```

For the full development reference (tests, lint, bundle-size budget, deployment workflow, release process), see [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

## Docker

ShellyAdmin runs as a single container. Tagged releases publish a multi-arch image to GHCR via `.github/workflows/publish-image.yml`:

- `ghcr.io/buliwyf42/shellyadmin:vX.Y.Z` (immutable)
- `ghcr.io/buliwyf42/shellyadmin:latest` (mover)

The reference Compose file at [`docker/docker-compose.yml`](docker/docker-compose.yml) pulls `:latest` by default. To build locally instead of pulling:

```bash
docker compose -f docker/docker-compose.yml up -d --build
```

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for the full deployment guide.

## Optional: MCP Server (Read-Only)

ShellyAdmin can expose a read-only [Model Context Protocol](https://modelcontextprotocol.io) server so LLM-driven agents (Claude Desktop, Claude Code, custom MCP clients) can introspect the fleet — list devices, check scan/firmware status, read compliance, inspect logs — without scraping the SPA. State-changing operations (refresh, scan, firmware update, provision, settings writes) are deliberately not exposed in v1; see [docs/adr/0011-mcp-read-only-server.md](docs/adr/0011-mcp-read-only-server.md).

The listener is **off by default** and can be enabled either by setting `SHELLYADMIN_MCP_TOKEN` (env var; takes precedence — useful for headless / CI / Compose-managed deploys) or by toggling **MCP Server → Enable** on the Settings page and entering a token there (since v0.1.20; encrypted at rest). When both are set, the env var wins and the Settings UI shows a "managed by environment variable" notice. Clients can authenticate either via the standard `Authorization: Bearer <token>` header **or** by putting the token as the first URL path segment (e.g. `http://host:8081/<token>/`) — convenient for clients like `mcp-remote` where a header arg is awkward.

| Env var                 | Default   | Purpose                                                                  |
| ----------------------- | --------- | ------------------------------------------------------------------------ |
| `SHELLYADMIN_MCP_TOKEN` | unset     | Required to enable MCP. Supports `_FILE` indirection like other secrets. |
| `SHELLYADMIN_MCP_PORT`  | `8081`    | Port for the MCP listener.                                               |
| `SHELLYADMIN_MCP_BIND`  | `0.0.0.0` | Bind address. Set to `127.0.0.1` for loopback-only.                      |

Example Claude Desktop config (`mcp.json`) — header form:

```json
{
  "mcpServers": {
    "shellyadmin": {
      "url": "http://your-shellyadmin-host:8081/",
      "headers": { "Authorization": "Bearer your-token-here" }
    }
  }
}
```

Same client routed through `mcp-remote` (which doesn't natively expose a header field) using the URL-path form:

```json
{
  "mcpServers": {
    "shellyadmin": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
        "http://your-shellyadmin-host:8081/your-token-here",
        "--allow-http"
      ]
    }
  }
}
```

When MCP is enabled, every tool call writes to the same audit log the SPA shows on the Logs page (prefixed with `mcp `, filterable by request id).

## Security

This project is intended for trusted LAN use, not direct internet exposure. See [SECURITY.md](SECURITY.md) for the reporting flow and supported-versions policy; [docs/SECURITY.md](docs/SECURITY.md) carries the deeper threat model and deployment expectations.

Found a vulnerability? Open a [private security advisory](https://github.com/buliwyf42/shellyadmin/security/advisories/new).

## Architecture

Documented in:

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — overall design
- [docs/adr/README.md](docs/adr/README.md) — architecture decision records

## Contributing

The project is still being shaped, so architecture changes should align with the documented design goals before implementation. See [CONTRIBUTING.md](CONTRIBUTING.md) for the development and PR workflow, and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community expectations.

## License

[MIT](LICENSE) © 2026 buliwyf42
