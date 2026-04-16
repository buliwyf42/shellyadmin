# ShellyAdmin — Developer Context

This file is a persistent memory aid for AI-assisted development. Keep it up to date when making architectural decisions.

---

## Architecture

- **Backend**: Go (single binary, `cmd/shellyadmin/main.go`)
- **Frontend**: Svelte + TypeScript SPA (`web/src/`)
- **Database**: SQLite via `modernc.org/sqlite` (no CGO required)
- **Deployment**: Multi-stage Docker image — Node 20 builds frontend, Go 1.24 builds backend, Alpine 3.19 is the runtime
- **Entry point**: `cmd/shellyadmin/main.go` → `internal/services/app.go`

The SPA is embedded into the Go binary at build time via `//go:embed`.

---

## Shelly Device Generations

| Gen | Protocol | Endpoint |
|-----|----------|---------|
| Gen1 | HTTP REST (GET with query params) | `/settings`, `/status`, `/reboot`, etc. |
| Gen2+ | JSON-RPC 2.0 (POST with JSON body) | `/rpc` |

Generation is detected via `GET /shelly` → `{"gen": N}`. Default to Gen2 if absent.

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

The provisioner's `ota` template section calls `OTA.SetConfig` which will always return a 404 from real devices (gracefully skipped). The `auto_update` field in the template is retained as policy-intent documentation, not an actual device setting.

### mqtt.ssl_ca valid values
The `mqtt.ssl_ca` field only accepts exactly four values:
- `""` / omitted — no TLS
- `"*"` — TLS, disable certificate validation
- `"ca.pem"` — TLS with built-in CA bundle
- `"user_ca.pem"` — TLS with user-uploaded CA certificate

### Time format (clock_mode)
- Gen1: `clock_mode` field in `/settings` (`0` = 24h, `1` = 12h)
- Gen2+: **no time format setting** — always 24h; the `time_format` compliance rule is silently skipped on Gen2+

### WS SSL CA
Same four-value pattern as MQTT: `""`, `"*"`, `"ca.pem"`, `"user_ca.pem"`.

---

## Key Files

| File | Role |
|------|------|
| `internal/services/app.go` | Service layer; job scheduling, refresh/scan orchestration |
| `internal/services/device_surface.go` | Bulk actions (set_sntp_server, etc.) |
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

| Section key | Gen1 | Gen2+ |
|-------------|------|-------|
| `sys` | `/settings` | `Sys.SetConfig` |
| `mqtt` | `/settings/mqtt` | `MQTT.SetConfig` |
| `ws` | skipped | `WS.SetConfig` |
| `ble` | skipped | `BLE.SetConfig` |
| `cloud` | skipped | `Cloud.SetConfig` |
| `matter` | skipped | `Matter.SetConfig` |
| `wifi` | skipped | `Wifi.SetConfig` |
| `auth` | skipped | `Shelly.SetAuth` |
| `ota` | skipped | `OTA.SetConfig` (404 on all devices → skipped) |
| `kvs` | skipped | `KVS.Set` per key |
| `gen2_rpc` | skipped | arbitrary method map |
| `gen1_http` | arbitrary endpoint map | skipped |
| anything else | skipped | `<Capitalized>.SetConfig` |

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

- `time_format` rule is **silently skipped** on Gen2+ (no such setting exists)
- `mqtt_connected` rule only applies to Gen2+ devices
- `cloud_enabled` checks the device's cloud enable setting (distinct from `cloud_connected`)
- Custom rules support `source: device | config | status`, path traversal with `.`, operators: `eq` (default), `ne`, `contains`, `regex`, `exists`
- `{device_name}` token in rule values is substituted with the device's effective name
