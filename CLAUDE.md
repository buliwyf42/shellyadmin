# ShellyAdmin — Developer Context

This file is a persistent memory aid for AI-assisted development. Keep it up to date when making architectural decisions.

---

## Architecture

- **Backend**: Go (single binary, `cmd/shellyctl/main.go`)
- **Frontend**: Svelte + TypeScript SPA (`web/src/`)
- **Database**: SQLite via `modernc.org/sqlite` (no CGO required)
- **Deployment**: Multi-stage Docker image — Node 20 builds frontend, Go 1.24 builds backend, Alpine 3.19 is the runtime
- **Entry point**: `cmd/shellyctl/main.go` → `internal/services/app.go`

The SPA is embedded into the Go binary at build time via `//go:embed`.

---

## Shelly Device Generations

Only Gen2+ devices are supported. Gen1 devices (HTTP REST / GET-based API) are not supported and will not be probed or provisioned.

| Gen | Protocol | Endpoint |
|-----|----------|---------|
| Gen2+ | JSON-RPC 2.0 (POST with JSON body) | `/rpc` |

Generation is detected via `GET /shelly` → `{"gen": N}`. Defaults to Gen2 if absent or zero.

---

## Shelly API Quirks

### Method-not-found error code
Shelly uses **non-standard JSON-RPC error code `404`** (not `-32601`) when a method is not supported on a specific device model. Example response:
```json
{"error": {"code": 404, "message": "Not Found"}}
```
`isMethodNotFound()` in `provisioner.go` handles both `404` and `-32601` for safety.

### OTA.SetConfig does not exist
The Shelly Gen2 API has **no `OTA.SetConfig` method**. Available OTA methods:
- `Shelly.Update` — one-shot firmware update (requires `stage` param: `"stable"` or `"beta"`)
- `Shelly.CheckForUpdate` — check for available updates

The `ota` provisioner section and the `ota_auto_update` compliance rule were **removed in v0.0.14**. The compliance check always fired "unsupported" because `ota.auto_update` is never present in any device config. The `ota` template section still exists in the catch-all handler (calls `OTA.SetConfig` → 404 → gracefully skipped) but is no longer exposed in the Provision form.

### mqtt.ssl_ca valid values
The `mqtt.ssl_ca` field only accepts exactly four values:
- `""` / omitted — no TLS
- `"*"` — TLS, disable certificate validation
- `"ca.pem"` — TLS with built-in CA bundle
- `"user_ca.pem"` — TLS with user-uploaded CA certificate

### WS SSL CA
Same four-value pattern as MQTT: `""`, `"*"`, `"ca.pem"`, `"user_ca.pem"`.

---

## Key Files

| File | Role |
|------|------|
| `internal/services/app.go` | Service layer; job scheduling, refresh/scan orchestration |
| `internal/services/device_surface.go` | Bulk actions (set_sntp_server, reboot, etc.) |
| `internal/core/scanner/scanner.go` | Device discovery & probing; populates `models.Device` |
| `internal/core/provisioner/provisioner.go` | Template-based fleet provisioning |
| `internal/core/compliance/compliance.go` | Compliance rule evaluation |
| `internal/core/setters/setters.go` | Targeted single-field setters for bulk actions |
| `internal/models/device.go` | Device struct (source of truth for all device fields) |
| `internal/models/settings.go` | ComplianceRules, AppSettings, etc. |
| `web/src/pages/Provision.svelte` | Provisioning UI — form editor + JSON editor |
| `web/src/pages/Compliance.svelte` | Compliance rules UI |

---

## Provisioner Template Sections

Sections in a template JSON map to backend handlers in `applySection()`:

| Section key | Handler |
|-------------|---------|
| `sys` | `Sys.SetConfig` |
| `mqtt` | `MQTT.SetConfig` |
| `ws` | `WS.SetConfig` |
| `ble` | `BLE.SetConfig` |
| `cloud` | `Cloud.SetConfig` |
| `matter` | `Matter.SetConfig` |
| `wifi` | `Wifi.SetConfig` (full surface: sta, sta1, roam, static IPv4) |
| `auth` | `Shelly.SetAuth` |
| `ota` | `OTA.SetConfig` (404 on all devices → skipped; form removed in v0.0.14) |
| `kvs` | `KVS.Set` per key |
| `script` | `Script.SetConfig` per id (loop like kvs) |
| `ui` | `UI.SetConfig` |
| `gen2_rpc` | arbitrary method map |
| `gen1_http` | skipped (legacy; Gen1 no longer supported) |
| anything else | `<Capitalized>.SetConfig` |

Template variable substitution: `{device_name}` is replaced with the device's configured name (from `Shelly.GetConfig` → `sys.device.name`).

---

## Deployment Workflow

All edits are made **locally on macOS**, then deployed to `docker.home.lan`:

```bash
# Sync code (exclude data/ — owned by container user)
rsync -av --exclude='data/' \
  "/Users/buliwyf/Documents/Codex + Code Projects/shellyadmin/" \
  buliwyf@docker.home.lan:/home/buliwyf/shellyadmin/

# On remote: rebuild and restart
ssh buliwyf@docker.home.lan "cd /home/buliwyf/shellyadmin && \
  docker build -t shellyadmin . && \
  docker stop shellyadmin && docker rm shellyadmin && \
  docker run -d --name shellyadmin \
    -p 8080:8080 \
    -v /docker/shellyadmin:/data \
    -e SHELLYADMIN_PASS=changeme \
    -e COOKIE_SECURE=false \
    shellyadmin"
```

The container uses a bind-mounted `data/` directory so SQLite persists across rebuilds.

The Dockerfile reads the `VERSION` file at the repo root as the default version when no `--build-arg VERSION=` is passed. This means local builds show the real version in the navbar and About page. **On each release, update both `VERSION` and `web/package.json` to the new version number.**

---

## Job Locking

Long-running jobs (refresh, scan, firmware_check) use a SQLite-backed status:
- `"running"` — job active
- `"done"` / `"failed"` — terminal
- `"interrupted"` — set on startup for any jobs stuck in `"running"` from a previous crash

A **stale-job guard** (2-minute timeout) prevents stuck `"running"` jobs from blocking manual triggers. Refresh jobs are **not** auto-restarted on startup (unlike scan/firmware_check) because they are user-initiated.

---

## Compliance Rules

Compliance rules in `models.ComplianceRules` are evaluated in `compliance.Evaluate()`. Key behaviors:

- `cloud_enabled` checks the device's cloud enable setting (distinct from `cloud_connected`)
- Custom rules support `source: device | config | status`, path traversal with `.`, operators: `eq` (default), `ne`, `contains`, `regex`, `exists`
- `{device_name}` token in rule values is substituted with the device's effective name
