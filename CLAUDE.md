# ShellyAdmin — Developer Context

This file is a persistent memory aid for AI-assisted development. Keep it up to date when making architectural decisions.

---

## Architecture

- **Backend**: Go 1.25 (single binary, `cmd/shellyctl/main.go`). The Go floor moved from 1.24 → 1.25 in v0.1.16 — gin v1.12.0 pulls `quic-go/quic-go` for HTTP/3, which requires Go 1.25.0 in its `go.mod`. CI is pinned to Go 1.25 in `.github/workflows/test.yml`; the Dockerfile backend stage uses `golang:1.25-alpine`.
- **Frontend**: Svelte + TypeScript SPA (`web/src/`)
- **Database**: SQLite via `modernc.org/sqlite` (no CGO required)
- **Deployment**: Multi-stage Docker image — Node 20 builds frontend, Go 1.25 builds backend, Alpine 3.19 is the runtime
- **Entry point**: `cmd/shellyctl/main.go` → `internal/services/app.go`

The SPA is embedded into the Go binary at build time via `//go:embed`.

### Dependency-bump trap (lesson from v0.1.14 / v0.1.15)

A bump of gin or any `golang.org/x/{net,text,sync}` to its latest version can silently raise the `go` directive in `go.mod` past whatever CI is running. v0.1.14 hit this: dep bumps pushed `go.mod` to `1.25.0` while CI was still on Go 1.24, breaking the Test + Publish-Image workflows. Two-line check before any dep bump:

```bash
go list -m -f "{{if .GoVersion}}{{.GoVersion}} {{.Path}}@{{.Version}}{{end}}" all 2>/dev/null | sort -V -r | head -10
grep -E "^go " go.mod
```

If anything in the top of that list outranks `go.mod`'s directive, either bump CI/Dockerfile/go.mod together (the v0.1.16 path) or pick an older version of the offending dep.

### MCP server (HTTP + stdio, opt-in)

Lives in `internal/mcp/`. Two transports share the same 21-tool surface:

- **HTTP** (default for remote access): `cmd/shellyctl/main.go` starts a second listener on `:8081` (configurable via `SHELLYADMIN_MCP_PORT` / `SHELLYADMIN_MCP_BIND`) that speaks Streamable HTTP MCP. Token comes from one of two sources, resolved in this order: (1) `SHELLYADMIN_MCP_TOKEN` env var (operator override, always wins; via `services.DecodeSecretValue` so `_FILE` indirection works), (2) `AppSettings.MCPEnabled && AppSettings.MCPToken != ""` from the persisted settings (added in v0.1.20). When neither is set, the listener does not bind. Authenticated by the resolved token via either `Authorization: Bearer <token>` header **or** a URL whose first path segment IS the token (e.g. `http://host:8081/<token>/` — same shape Home Assistant uses, ergonomic for `mcp-remote`-style clients). Both checks run through `subtle.ConstantTimeCompare`; the matched path prefix is stripped before reaching the SDK handler.
- **Stdio** (v0.2.3+, for Claude Desktop on the same host): `shellyctl mcp` subcommand. `cmd/shellyctl/mcp_stdio.go` opens the database, builds a minimal AppService (no background workers — query session, not server), and serves over `mcp.StdioTransport` via `internal/mcp.RunStdio`. No transport-level auth — the parent process spawning the binary IS the trust boundary; host filesystem permissions on the data dir are the remaining gate. Logs to stderr; stdout carries JSON-RPC frames. SQLite WAL mode handles concurrent readers if a long-running HTTP-mode container shares the same data dir.

- **Surface (v0.2.3)**: 21 tools. **13 read-only**: list_devices, get_device, list_device_actions, scan_status, firmware_status, firmware_install_status, list_templates, get_template, list_credentials, get_settings, get_logs, export_device, compliance_summary. **8 state-changing, all confirm-gated**: refresh_device, refresh_all_devices, start_scan, confirm_scan, firmware_check, firmware_install, execute_device_action, bulk_action. All thin adapters over `services.AppService`. Hard exclusion: anything that mutates ShellyAdmin's *own* config (save_settings, save_credential, save_template, provision, clear_logs).
- **`firmware_status` paging (v0.2.3)**: optional `status` / `has_update` / `search` / `limit` / `offset` inputs; output adds `filtered_total` (post-filter) and `returned` (post-page) alongside the unchanged `running` / `done` / `total` job-level metrics. Matters past ~200 devices where the unfiltered payload approaches MCP per-tool output caps.
- **Confirm-flow contract** (added v0.1.22, see `internal/mcp/tools_actions.go` `confirmPolicy`): every state-changing tool has a `Confirm bool` input. Without `confirm: true` the tool returns a typed preview (`SimpleActionResult.Preview=true` + per-tool fields like target counts, risk levels, per-target eligibility from `PreviewBulkAction`) and does NOT call the underlying AppService method. With `confirm: true` it executes. Each call audit-logs `mode=preview` or `mode=confirmed` so operators can pair them by request_id. `actionTool` wraps the context with `services.WithRisk(ctx, "low|medium|high")` so audit rows carry `risk_level`. The tool description includes a verbatim "OPERATOR APPROVAL REQUIRED" policy paragraph telling the LLM to summarize and ask before passing confirm=true.
- **Secret hygiene**: `list_credentials` and `get_settings` route through `internal/mcp/redact.go`. Plaintext password and HA1 hashes never leave the process via MCP. New fields with secret material must add a redactor before they're exposed.
- **Audit**: every tool call logs through `service.LogCtx(ctx, ...)`; `X-Request-ID` is honored on the request and echoed back. Audit rows show in `/api/logs` with `mcp ` prefix, filterable by request_id.
- **Why a separate port** (not `/mcp` on `:8080`): the MCP auth path stays off the cookie + CSRF middleware chain that protects the SPA, and an MCP listener bind failure is isolated from the main UI.
- **`scan_status` returns slim pending entries** (`{mac, ip, name, model, gen, app}` only, not full `models.Device`) — full payload was ~63 KB on a 44-device fleet and tripped MCP client output caps. The SPA shape is unchanged. If you add another tool that returns lists keyed off `models.Device`, follow the same pattern (`internal/mcp/tools.go` `slimScanPending` / `ScanPendingItem`).
- **Target resolution** for `get_device` / `list_device_actions` / `export_device` accepts MAC, IP, **or device name** (`services.GetDeviceDetail`). Don't reintroduce a MAC/IP-only check there — the tool descriptions advertise all three.
- **MCP token in settings is encrypted at rest** via `internal/core/secretbox`. `services.SaveSettings` seals; `services.GetSettings` opens. The API GET handler in `internal/api/handler.go` re-redacts to `services.MCPTokenRedacted` (`"<set>"`) before sending to the SPA. When the SPA round-trips settings unchanged, sending `"<set>"` back means "keep the existing token" (the magic value to preserve, not a literal token). Don't expose plaintext `MCPToken` over any new API surface — add a redactor first.
- **MCP listener lifecycle is live** as of v0.1.21. `services.AppService` owns an `*MCPController` (`internal/services/app_mcp.go`) holding the `*http.Server`. `SaveSettings` calls `ReconcileMCPFromSettings` after persisting, which serializes start/stop/rotate transitions on a controller-local mutex. Env-locked instances ignore reconcile. `cmd/shellyctl/main.go` injects `mcp.Build` as the builder via `SetMCPParams` to avoid a services↔mcp import cycle. `Stop(ctx)` tears the listener down before draining background workers. **One AppService is shared between main.go and the API handlers** via `api.Config.Service` — don't reintroduce a second `services.NewAppService(...)` inside the handler, the controller's live state would split.
- **`api.Config.Service`** is the way the api package consumes the shared AppService. When set, NewHandler reuses it; when nil, NewHandler still falls back to constructing its own (kept for tests that don't need to share state). main.go always sets it.

See [docs/adr/0011-mcp-read-only-server.md](./docs/adr/0011-mcp-read-only-server.md) for the full design rationale, the v0.1.22 state-changing tools addendum, and the v0.2.3 stdio + paging follow-up.

---

## Shelly Device Generations

Only Gen2+ devices are supported. Gen1 devices (HTTP REST / GET-based API) are not supported and will not be probed or provisioned.

| Gen   | Protocol                           | Endpoint |
| ----- | ---------------------------------- | -------- |
| Gen2+ | JSON-RPC 2.0 (POST with JSON body) | `/rpc`   |

Generation is detected via `GET /shelly` → `{"gen": N}`. Defaults to Gen2 if absent or zero.

---

## Shelly API Quirks

### Method-not-found error code

Shelly uses **non-standard JSON-RPC error code `404`** (not `-32601`) when a method is not supported on a specific device model. Example response:

```json
{ "error": { "code": 404, "message": "Not Found" } }
```

`isMethodNotFound()` in `provisioner.go` handles both `404` and `-32601` for safety.

### OTA configuration on Gen2+ — implemented via `Schedule.*`, not `OTA.SetConfig`

The Shelly Gen2 API has **no `OTA.SetConfig` / `Sys.SetAutoUpdate` / dedicated OTA-config method**. The `OTA.*` methods that DO exist (`OTA.Start/Write/Data/Abort/Commit/Revert`) are byte-level chunked-upload plumbing, not configuration. Direct firmware update lives at:

- `Shelly.Update` — one-shot firmware update (requires `stage` param: `"stable"` or `"beta"`)
- `Shelly.CheckForUpdate` — check for available updates (returns BOTH `stable` and `beta` in one response)

**Auto-update is implemented as a Schedule entry.** The device's local web UI ("Enable auto update firmware", added in firmware 1.2.0) does NOT call a dedicated method. Instead it creates a `Schedule.*` job that calls `Shelly.Update` on a recurring timer with `origin: "shelly_service"` as the marker. ShellyAdmin reads/writes auto-update state through this same mechanism (see `internal/core/firmware/autoupdate.go`):

- **Read**: `Schedule.List` → filter for `calls[].origin == "shelly_service"` AND `calls[].method == "Shelly.Update"`. The `params.stage` field tells you `stable` or `beta`. Absent or disabled → `off`.
- **Set stable/beta**: `Schedule.Create` with `enable: true`, `timespec: "0 0 0 * * 0,1,2,3,4,5,6"` (cron-style; daily at midnight), `calls: [{method: "Shelly.Update", params: {stage: <stable|beta>}, origin: "shelly_service"}]`.
- **Disable**: `Schedule.Delete` for the matching job id.

Persisted on the Device row as `fw_auto_update` with values `""` (never read) | `off` | `stable` | `beta`. Read during every firmware check job. Bulk-settable via the Firmware page's "Auto → Off / Stable / Beta" buttons (action `set_auto_update`). Surfaced in compliance via the `auto_update_stage` rule.

Historical context: the `ota` provisioner section and an `ota_auto_update` compliance field that called `OTA.SetConfig` were **fully removed in v0.0.16** (the v0.0.14 removal was partial). If an `ota` block still appears in a user-supplied JSON template, it falls through to the catch-all handler (calls `Ota.SetConfig` → 404 → gracefully skipped).

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

| File                                       | Role                                                                          |
| ------------------------------------------ | ----------------------------------------------------------------------------- |
| `internal/services/app.go`                 | Service layer; job scheduling, refresh/scan orchestration                     |
| `internal/services/device_surface.go`      | Bulk actions (set_sntp_server, reboot, etc.)                                  |
| `internal/core/scanner/scanner.go`         | Device discovery & probing; populates `models.Device`                         |
| `internal/core/firmware/firmware.go`       | `Shelly.GetDeviceInfo` + `Shelly.CheckForUpdate` per channel; install trigger |
| `internal/core/firmware/autoupdate.go`     | `Schedule.*`-based auto-update read/write (see ADR-0009)                      |
| `internal/core/firmware/methods.go`        | `Shelly.ListMethods` capability probe (see ADR-0010)                          |
| `internal/core/provisioner/provisioner.go` | Template-based fleet provisioning                                             |
| `internal/core/compliance/compliance.go`   | Compliance rule evaluation                                                    |
| `internal/core/setters/setters.go`         | Targeted single-field setters for bulk actions                                |
| `internal/core/clock/clock.go`             | Tiny `Clock` interface + `Real()` + `Fake.Advance(d)` for deterministic tests |
| `internal/core/secretbox/secretbox.go`     | NaCl secretbox envelope encryption for credential at-rest storage             |
| `internal/middleware/requestid.go`         | `X-Request-ID` middleware; IDs propagate to audit_log rows and slog attrs     |
| `internal/services/password.go`            | Argon2id hash/verify for `SHELLYADMIN_PASS_HASH`                              |
| `internal/services/store.go`               | `Store` interface at the service/DB boundary                                  |
| `internal/api/errors.go`                   | `respondError` / `respondUserError` — sanitized HTTP error responses          |
| `internal/models/device.go`                | Device struct (source of truth for all device fields)                         |
| `internal/models/settings.go`              | ComplianceRules, AppSettings, etc.                                            |
| `internal/mcp/server.go`                   | `Build` — HTTP MCP listener (token-gated, request-id middleware)              |
| `internal/mcp/stdio.go`                    | `RunStdio` — same tool surface over `mcp.StdioTransport` (v0.2.3+)            |
| `internal/mcp/tools.go`                    | Read-only tools + filter/page helpers (firmware_status, list_devices, etc.)   |
| `internal/mcp/tools_actions.go`            | Confirm-gated state-changing tools + `actionTool` audit wrapper (v0.1.22+)    |
| `cmd/shellyctl/mcp_stdio.go`               | `shellyctl mcp` subcommand entry — minimal AppService, stderr-only logs      |
| `web/src/pages/Provision.svelte`           | Provisioning UI — form editor + JSON editor                                   |
| `web/src/pages/provision/`                 | Section forms: Sys, Mqtt, Ws, Ble, Wifi, Eth, Modbus, Zigbee, Scripts, Webhooks (v0.2.4), Cover (v0.2.5), ZigbeeOps (v0.2.6), UserCA |
| `web/src/pages/Compliance.svelte`          | Compliance rules UI                                                           |

---

## Provisioner Template Sections

Sections in a template JSON map to backend handlers in `applySection()`:

| Section key   | Handler                                                                                  |
| ------------- | ---------------------------------------------------------------------------------------- |
| `sys`         | `Sys.SetConfig`                                                                          |
| `mqtt`        | `MQTT.SetConfig`                                                                         |
| `ws`          | `WS.SetConfig`                                                                           |
| `ble`         | `BLE.SetConfig`                                                                          |
| `cloud`       | `Cloud.SetConfig`                                                                        |
| `matter`      | `Matter.SetConfig`                                                                       |
| `wifi`        | `Wifi.SetConfig` (full surface: sta, sta1, roam, static IPv4)                            |
| `auth`        | `Shelly.SetAuth`                                                                         |
| `ota`         | catch-all handler; Shelly returns 404 → `skipped` (form + normalizer removed in v0.0.16) |
| `kvs`         | `KVS.Set` per key                                                                        |
| `script`      | `Script.SetConfig` per id (loop like kvs)                                                |
| `ui`          | `UI.SetConfig`                                                                           |
| `gen2_rpc`    | arbitrary method map                                                                     |
| `gen1_http`   | skipped (legacy; Gen1 no longer supported)                                               |
| anything else | `<Capitalized>.SetConfig`                                                                |

Template variable substitution: `{device_name}` is replaced with the device's configured name (from `Shelly.GetConfig` → `sys.device.name`).

---

## Deployment Workflow

All edits are made **locally on macOS**, then deployed to `docker.home.lan`:

```bash
# Sync code (exclude data/ — owned by container user)
rsync -av --exclude='data/' \
  "/Users/buliwyf/Documents/Codex + Code Projects/shellyadmin/" \
  buliwyf@docker.home.lan:/home/buliwyf/shellyadmin/

# On remote: rebuild and restart. Generate the hash once with
#   docker run --rm shellyadmin hash-password <plaintext>
# and substitute the PHC string for $HASH below.
ssh buliwyf@docker.home.lan "cd /home/buliwyf/shellyadmin && \
  docker build -t shellyadmin . && \
  docker stop shellyadmin && docker rm shellyadmin && \
  docker run -d --name shellyadmin \
    -p 8080:8080 \
    -v /docker/shellyadmin:/data \
    -e SHELLYADMIN_PASS_HASH='\$HASH' \
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

---

## Testability Pattern: OnClient Seams + Clock

Added in v0.1.15 (M3a). Each device-talking package (`internal/core/{scanner,firmware,setters}`) ships in two layers:

- **Public `…WithOptions` / `New(opts)` entry points** — production callers use these. They build a `*shellyclient.Client` from `Options` and delegate to the seam below. Behavior unchanged from before M3.
- **`…OnClient` seams** — accept a pre-built `*shellyclient.Client` directly. The precedent is `firmware/methods.go:30 ListSupportedMethodsOnClient`; v0.1.15 brought scanner / firmware / setters into line. Tests construct a `httptest.NewServer` fake-Shelly + a `shellyclient.Client` aimed at it, then call the OnClient variant.

`scanner.ProbeOptions` and `firmware.Options` carry an optional `Clock clock.Clock` field — nil falls back to `clock.Real()`. Tests inject `clock.NewFake(t)` + `Advance(d)` to pin timestamp-bearing fields (`LastSeen`, `AuthLockedUntil`, `CheckedAt`) to deterministic values.

When you add a new device-talking call site, follow the same pattern:

1. Public `…WithOptions` builds the client from `Options`.
2. Internal `…OnClient(ctx, client, …)` does the work.
3. Any wall-clock dependency goes through `clk.Now()`, not `time.Now()`.

The shared test fixture for firmware lives at `internal/core/firmware/helpers_test.go` (`fakeShelly` — per-method handler map, call recorder, defaults to the Shelly non-standard 404 RPC error for unregistered methods). Reuse it for new firmware tests.

---

## App Settings (operator-facing)

Defined in `models.AppSettings` (`internal/models/settings.go`), normalised on load via `Normalize()`. Notable knobs:

| Field                         | JSON key                         | Default     | Bounds      | Notes                                                                                         |
| ----------------------------- | -------------------------------- | ----------- | ----------- | --------------------------------------------------------------------------------------------- |
| `FirmwareInstallTimeout`      | `firmware_install_timeout`       | 300 (5 min) | `> 0`       | Per-device cap before install_job marks "unknown"                                             |
| `FirmwareInstallPollInterval` | `firmware_install_poll_interval` | 5           | `[1, 60]` s | How often the install_job re-queries device firmware while waiting for reboot (added v0.1.13) |
| `FirmwareCheckInterval`       | `firmware_check_interval`        | 0 (off)     | `≥ 0` s     | Periodic firmware_check job cadence; 0 disables the scheduler                                 |

The pattern for adding a new knob:

1. Field on `AppSettings` with JSON tag.
2. Default in `DefaultSettings()`.
3. Bounds clamp in `Normalize()`.
4. A `…FromSettings(s) <Type>` helper in the consuming service (see `firmwareInstallTimeoutFromSettings` / `firmwareInstallPollIntervalFromSettings` in `internal/services/app_jobs.go`).
5. Settings.svelte input — match `firmware_install_timeout`'s number-input shape unless a preset dropdown fits better.
6. TS field on `AppSettings` in `web/src/lib/types.ts`.

---

## Plaintext Password Removed

`SHELLYADMIN_PASS` (plaintext) was removed in **v0.2.0**. v0.0.15 added `_HASH` and started warning on plaintext use; v0.2.0 closed the deprecation window. The only supported entry point is `SHELLYADMIN_PASS_HASH` (or `_FILE` indirection) carrying an argon2id PHC string from `shellyctl hash-password`. Missing it panics at startup. `internal/services/password.go` and the `shellyctl hash-password` subcommand are unchanged — they were the migration target, not part of the removal.

---

## Release Cadence Convention

VERSION + `web/package.json` + lockfile bump together on every release. Tag is lightweight (`git tag vX.Y.Z`, no `-a`); push needs `git push origin main vX.Y.Z` because `--follow-tags` only auto-pushes annotated tags. CHANGELOG header convention is `## [X.Y.Z] - YYYY-MM-DD — em-dash subtitle`; the publish-image workflow extracts the subtitle for the auto-created GitHub Release title.
