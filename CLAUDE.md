# ShellyAdmin — Developer Context

This file is a persistent memory aid for AI-assisted development. Keep it up to date when making architectural decisions.

For deployment workflow, release cadence, and CI/branch-protection
details, see [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

---

## Architecture

- **Backend**: Go 1.25 (single binary, `cmd/shellyctl/main.go`). The Go floor moved from 1.24 → 1.25 in v0.1.16 — gin v1.12.0 pulls `quic-go/quic-go` for HTTP/3, which requires Go 1.25.0 in its `go.mod`. CI's `setup-go` and the Dockerfile backend stage use the **Go 1.26 toolchain** (bumped in v0.3.4); the `go.mod` directive stays `go 1.25.0` — the floor — and the newer toolchain builds it backward-compatibly. **The binary links quic-go but does NOT open any UDP/QUIC listener** — the HTTP server in main.go is plain `net/http` over TCP. Phase 2 (v0.2.11) verified this; the QUIC code paths in the binary are dead weight at runtime, not an attack surface.
- **Frontend**: Svelte + TypeScript SPA (`web/src/`)
- **Database**: SQLite via `modernc.org/sqlite` (no CGO required)
- **Deployment**: Multi-stage Docker image — Node 26 builds frontend, Go 1.26 builds backend, Alpine 3.24 is the runtime (bumped from 3.23 in v0.5.5)
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

### Firmware 2.0.0 auth — brute-force protection → `429`

Firmware **2.0.0** (2026-07-13) added device-side brute-force protection plus RFC-7616-compliant nonce management. Fleet operations (`bulk_action`, `refresh_all_devices`) run against a device with **wrong or stale credentials** can trip a per-device lockout, surfaced as HTTP `429 Too Many Requests`. No code fix is needed — `shellyclient` is already 2.0.0-shaped: RFC-7616 Digest with per-client nonce reuse + `nc` counter, a **stale-nonce retry** (re-parses the fresh challenge on a second 401, `client.go:316-358`), and `429` pass-through (`client.go:355`). Operational takeaway only: repeated wrong-credential attempts across the fleet will lock devices, so fix the credential before re-running a bulk job.

### OTA failures: `premature end of data` — cause UNKNOWN, do not guess again

Fleet OTAs to 2.0.0 fail at random progress percentages with a device-side error:

```
hos_http_client.cpp: Finished; bytes 326/45056, code 200, status -1: Connection error: -14
shos_ota.cpp:650     Update failed (DATA_LOSS: ZIP flush error : premature end of data), will not reboot
```

The device's download from `fwcdn.shelly.cloud` truncates. **The root cause is not known.**
What is established (2026-07-17):

- **Not the CDN.** The same URL fetched from the LAN returns the full image, 3× in a row:
  3,814,926 bytes, `Content-Length` matches, valid zip.
- **Not ShellyAdmin's polling.** Same device (`.129`, RSSI −58), same firmware: failed at 12%
  while polled every 5s, failed at 37% with no polling at all — identical error both times.
- **Signal is a factor, not the whole story.** `.59` sits at **−78 dBm** (the device's own
  roam threshold is −80) and fails early plus times out on ordinary RPCs. But `.129` at
  −58 dBm fails too.
- **It is not universal.** `.92` (−50 dBm) completed 0→100% in 2:25 once, and four devices
  self-updated overnight via Shelly's phased cloud rollout.
- **Untested lead:** curl on a LAN host could not verify `fwcdn.shelly.cloud`'s certificate
  ("unable to get local issuer certificate"), which would fit TLS interception somewhere in
  the path. The devices pin `shelly_cloud.pem`. Unverified.

**A prior version of this section claimed the cause was ShellyAdmin polling the device
during the download. That was wrong** — it rested on comparing two *different* devices
(`.92` unpolled vs `.59` polled) and attributing the difference to polling, when `.59` also
has the worst signal in the fleet and fails unpolled. v0.5.6 shipped that claim in its
CHANGELOG; v0.5.7 retracted it. If you are tempted to explain these failures from one
suggestive pair of runs, don't — get the A/B on one device first.

**Gotcha for any future measurement:** the Docker host is multi-homed and the production
ShellyAdmin reaches the IoT VLAN as **`192.168.211.88`**, polling every device roughly every
60s. An "unpolled" experiment against the live fleet is not unpolled unless that instance is
stopped. Check the source IPs in the device log before trusting a result.

**Diagnostic:** `curl -N http://<ip>/debug/log` streams the device's log as plain text over
HTTP — no config change, no MQTT/websocket detour. It carries `ota_begin` / `ota_progress` /
`ota_success` / `ota_error` events and the resolved `fwcdn.shelly.cloud` URL, and is by far
the fastest way to see what an OTA is actually doing.

`FirmwareInstallQuietPeriod` (default 150s) keeps the job off the device between trigger and
reboot. Keep it — the version cannot change during the download, so polling then buys
nothing — but it is hygiene, **not** a fix for the failures above.

### Update availability is a version comparison, not a string compare

`firmware.IsNewer` (x/mod/semver, `internal/core/firmware/firmware.go`) decides
`Result.StableUpdate` / `BetaUpdate`. It must never go back to `!=`: a device on a beta sits
**ahead** of its model's stable channel, so string inequality advertises the older stable as
an available update. During the phased 2.0.0 rollout that mislabelled **36 of 44** fleet
devices (on `2.0.0-beta3`, offered stable `1.7.5` / `1.7.99-powerstripg4prod1` /
`1.8.99-plugmg3prod0`), and each triggered install was silently ignored by the device.

Shelly versions are semver-shaped, so prerelease < release gives `2.0.0-beta3 < 2.0.0`
correctly. Unparseable versions fall back to string inequality — an odd vendor string can
fail to suppress a downgrade, but can never *hide* a real update.

**The rollout is phased per device, not per model** — do not diagnose it as a device fault.
Two identical `S4PL-00416EU` strips, checked the same minute (2026-07-21): `shelly-strip4-01`
was offered stable `2.0.0`, `shelly-strip4-02` still stable `1.7.99-powerstripg4prod1`
(build `20250819`, the factory image). Same for their `alt` `PowerStripZB` entry. Ground
truth is the device's own `Shelly.CheckForUpdate` — the server answers per device id, so a
missing update on one unit says nothing about the model. Nothing to fix; wait for the bucket.

### The firmware index: `updates.shelly.cloud` — and why it can't shortcut a phased rollout

Contrary to the forum consensus that Gen2+ has no offline update path, there **is** a public index:

```
curl -k https://updates.shelly.cloud/update/<APP>     # APP = Shelly.GetDeviceInfo → "app", e.g. Mini1PMG3
→ {"stable":{"version","build_id","url"},"beta":{…},"alt":…,"time":…}
```

`-k` is required — `updates.` and `fwcdn.shelly.cloud` serve certificates from Allterco's internal CA, so
curl fails with a bare `https://` (exit 60, and a *silent* `000` if you don't check the status). The `url`
points at `https://fwcdn.shelly.cloud/gen2-ntest/<APP>/<sha256>` and serves the ZIP to a LAN host fine
(verified 2026-07-22: `200`, `Content-Type: application/zip`, 3,435,534 B for Mini1PMG3 beta3). That image
can be handed to a device via `Shelly.Update{"url": …}` — the one param ShellyAdmin does **not** send
(`TriggerUpdateOnClient` passes only `stage`).

**But the index carries only the broadly-published builds, not the rollout bucket.** Checked 2026-07-22
across all 12 fleet apps while 31 of 44 devices already ran stable `2.0.0` (`20260710-101127/2.0.0-g87fbfa4`):
**every** app still advertised stable `1.7.5` (or the model's factory build) and beta `2.0.0-beta3`. 2.0.0
stable appeared for **no app at all**. Device-identifying query params (`id`, `uid`, `mac`, `device_id`,
`ver`) do not change the answer — the bucketing happens elsewhere. So a `url` install cannot pull a device
forward into a rollout it hasn't been assigned to; every reachable URL is either a downgrade or the build
it already runs. **Do not re-derive this.** Wait for the bucket.

Where the index *is* worth using: it's the missing piece for the `premature end of data` failures above —
fetch the ZIP once to a LAN host, serve it (`python3 -m http.server`), point `Shelly.Update{url}` at plain
`http://`, and the device never touches `fwcdn.shelly.cloud`. That sidesteps the whole untested TLS-
interception lead. Not yet tried against a real failure — there has been nothing to install since.

Second, unrelated use: **the index dates EOL hardware without trusting a vendor blog post.** `Plus1` and
`Plus2PM` are the only fleet apps with **no `beta` key at all**, while every Gen3/Gen4 app carries
`2.0.0-beta3` — independent confirmation that the Gen2 Plus line is frozen at 1.7.5.

### OTA configuration on Gen2+ — implemented via `Schedule.*`, not `OTA.SetConfig`

The Shelly Gen2 API has **no `OTA.SetConfig` / `Sys.SetAutoUpdate` / dedicated OTA-config method**. The `OTA.*` methods that DO exist (`OTA.Start/Write/Data/Abort/Commit/Revert`) are byte-level chunked-upload plumbing, not configuration. Direct firmware update lives at:

- `Shelly.Update` — one-shot firmware update (requires `stage` param: `"stable"` or `"beta"`)
- `Shelly.CheckForUpdate` — check for available updates (returns BOTH `stable` and `beta` in one response)

**Auto-update is implemented as a Schedule entry.** The device's local web UI ("Enable auto update firmware", added in firmware 1.2.0) does NOT call a dedicated method. Instead it creates a `Schedule.*` job that calls `Shelly.Update` on a recurring timer with `origin: "shelly_service"` as the marker. ShellyAdmin reads/writes auto-update state through this same mechanism (see `internal/core/firmware/autoupdate.go`):

- **Read**: `Schedule.List` → filter for `calls[].origin == "shelly_service"` AND `calls[].method == "Shelly.Update"`. The `params.stage` field tells you `stable` or `beta`. Absent or disabled → `off`.
- **Set stable/beta**: `Schedule.Create` with `enable: true`, `timespec: "0 0 0 * * 0,1,2,3,4,5,6"` (cron-style; daily at midnight), `calls: [{method: "Shelly.Update", params: {stage: <stable|beta>}, origin: "shelly_service"}]`.
- **Disable**: `Schedule.Delete` for the matching job id.

**`stage` is a slot, not a floor.** `stage: "beta"` installs whatever the server lists under
`beta` — it does **not** mean "beta or anything newer". A device parked on `2.0.0-beta3` with
`stage: "beta"` is a permanent no-op (its beta slot already matches) and will *not* pick up
`2.0.0` when that lands, because the final release ships in the **stable** slot. For a device
sitting ahead of its own stable channel during a phased rollout, `stage: "stable"` is the
correct setting: it no-ops while the offered stable is older (the device ignores the install,
see the rollout note above) and installs the new build the moment the rollout reaches it.

Persisted on the Device row as `fw_auto_update` with values `""` (never read) | `off` | `stable` | `beta`. Read during every firmware check job. Bulk-settable via the Firmware page's "Auto → Off / Stable / Beta" buttons (action `set_auto_update`). Surfaced in compliance via the `auto_update_stage` rule.

### Alternative firmware variants — `sys.alt` (firmware 2.0.0+), read-only

Firmware 2.0.0-beta3 added an `alt` object: alternative firmware **variants** for the same hardware — a Zigbee or Matter build, or an add-on profile (e.g. Power Strip Gen4 → `PowerStripZB` "with Zigbee", Mini 1PM Gen4 → `Mini1PMG4ZB`, Pro 3EM → `Pro3EMProAddon`). It is a **map** keyed by variant id → `{name, desc, stable?{version,build_id}, beta?{version,build_id}}`, NOT a scalar third channel.

**Source is `Shelly.GetStatus` → `sys.alt`**, not `Shelly.CheckForUpdate` — a deliberate choice, not the only option: as of firmware **2.0.0 stable** (2026-07-13) the `alt` object *is* also carried in `Shelly.CheckForUpdate` (during the betas it lived only in sys status, so the earlier note that "CheckForUpdate stayed stable+beta only" is now outdated). We keep reading `sys.alt` because it's free — the scanner already fetches `Shelly.GetStatus` into `Device.RawStatus`, so **no extra RPC and no new DB column** — `sysAltVariants()` in `internal/services/actions.go` derives `Device.FWAlt []models.AltFirmwareVariant` from the cached RawStatus at `GetDevices()` time, exactly like `SwitchCount`. Surfaced in `/api/devices` (DeviceListView) + MCP `get_device`/`list_devices`, and as an `alt: <id>` badge in the Model cell of the Firmware page.

**Read-only, by design.** `Shelly.Update` accepts only `stage` (stable|beta) or `url` — there is **no `stage:"alt"`** and the alt object carries **no `url`**. So a variant/protocol switch (e.g. flashing a plug to Zigbee firmware) is NOT wired and can't be, with the currently documented API. ShellyAdmin only *shows* which devices could switch (useful for the ZHA fleet); the actual switch is done via the device's own web UI. If Shelly later documents an install path, wiring lives at the `TriggerUpdate*` seam in `internal/core/firmware/firmware.go`.

`sys.provisioning` (secure-provisioning state, same firmware) rides the same RawStatus read: `sysProvisioning()` → `Device.Provisioning map[string]any`, surfaced in `get_device`. Absent fleet-wide until a device is enrolled in secure provisioning.

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
| `FirmwareInstallTimeout`      | `firmware_install_timeout`       | 600 (10 min) | `> 0`, and `≥ quiet + 150` | Per-device cap before install_job marks "unknown". Normalize forces the floor so the timeout can't expire before the first poll |
| `FirmwareInstallQuietPeriod`  | `firmware_install_quiet_period`  | 150         | `(0, 600]` s; 0 = unset → default | How long install_job leaves the device alone after the trigger. Hygiene, **not** a fix — see the OTA-failure section above (v0.5.6 claimed otherwise, v0.5.7 retracted) |
| `FirmwareInstallPollInterval` | `firmware_install_poll_interval` | 5           | `[1, 60]` s | How often the install_job re-queries device firmware **after** the quiet period (added v0.1.13)                                               |
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

`SHELLYADMIN_PASS` (plaintext) was removed in **v0.2.0**. v0.0.15 added `_HASH` and started warning on plaintext use; v0.2.0 closed the deprecation window. The argon2id hash/verify in `internal/services/password.go` and the `shellyctl hash-password` subcommand remain — but as of first-run setup (below) `SHELLYADMIN_PASS_HASH` is no longer the *source of truth* for the login, only a one-time import seed.

---

## First-Run Setup — operator login in the DB (ADR-0017)

The operator login (username + argon2id hash) lives in the database, not the environment. See [docs/adr/0017-first-run-setup.md](./docs/adr/0017-first-run-setup.md).

- **Storage**: single-row `admin_credentials` table (migration 031), accessed via `db.{Get,Save,Clear}AdminCredential` and the service helpers in [internal/services/app_auth.go](internal/services/app_auth.go). NOT in `AppSettings` (which is mirrored to the SPA via `GET /api/settings` — a hash must never go there). The PHC hash is one-way, so it is stored verbatim, NOT secretbox-sealed.
- **Resolution**: the login handler resolves the credential at request time via `h.adminCredential()` ([internal/api/handler.go](internal/api/handler.go)) — DB first, then a `cfg.User`/`cfg.PassHash` fallback kept so handler tests that seed `Config` (not the DB) still pass. The lockout/TOTP keys use the *resolved* username.
- **Boot ([cmd/shellyctl/main.go](cmd/shellyctl/main.go))**: the old "panic when `SHELLYADMIN_PASS_HASH` is empty" is GONE. `ImportEnvCredentialOnce(user, passHash)` imports a still-present env hash into the DB exactly once (only when no DB credential exists) — seamless upgrade for existing deployments. With no credential at all the server logs a setup-mode warning and boots anyway.
- **Setup mode**: no credential ⇒ the SPA renders the public setup screen (`/setup`) gated by `GET /api/setup/status` → `{configured: bool}`. `POST /api/setup` is the **only unauthenticated mutation** — public, rate-limited, one-shot (409 once configured), race-guarded by a service mutex + the single-instance lock.
- **Change later**: `POST /api/account/credentials` (authenticated, **cookie-only** — a PAT cannot rotate the login that gates it). Verifies the current password, updates, then revokes all sessions (SPA redirects to `/login`). UI: `web/src/pages/settings/AccountCard.svelte`.
- **Recovery**: `shellyctl reset-auth --force` clears the row → next boot is setup mode again (mirrors `shellyctl unlock --force`). This is the forgotten-password path now that env is no longer authoritative.
- The encryption-key requirement (next section) is unaffected and still enforced — setup mode still needs `SHELLYADMIN_ENCRYPTION_KEY`.

---

## Encryption Key Required (v0.3.0)

S6 from the consolidated review (ADR-0013) closed the encryption-key auto-generation path in v0.3.0. The boot path's `loadEncryptionKey()` in [cmd/shellyctl/main.go](cmd/shellyctl/main.go) refuses to start when neither `SHELLYADMIN_ENCRYPTION_KEY` nor `SHELLYADMIN_ENCRYPTION_KEY_FILE` is set. v0.2.11 added the deprecation warning; v0.3.0 turned it into a hard error.

Migration recipe for operators upgrading from v0.2.x with an auto-generated `{dataDir}/shellyadmin.key`: copy the file contents to a path outside the data volume (Docker secret, NixOS secret store, sops-encrypted file in the homelab config repo), then set `SHELLYADMIN_ENCRYPTION_KEY_FILE=/path/to/it` in the compose `.env`. The startup error message includes the legacy path when it's detected, so the recovery is one `cat` away.

Threat closed: a volume snapshot exfiltrating both the encrypted credentials in `shellyctl.db` AND the key file sitting next to it — defeated the at-rest encryption entirely. External key management = both halves no longer share a backup boundary.

**Key rotation (v0.5.3)**: `shellyctl rotate-key` re-seals every secretbox blob — credentials, credential groups, TOTP material, the MCP token inside the settings JSON — in ONE transaction (`internal/db/rotate.go` `RotateSealedColumns`). Old key from `SHELLYADMIN_ENCRYPTION_KEY[_FILE]`, new key from `SHELLYADMIN_NEW_ENCRYPTION_KEY[_FILE]`; without `--force` it's a dry run that verifies the old key opens everything. Writes a timestamped DB backup before applying; refuses while a fresh runtime-lock heartbeat exists (live server). Explicit-key seal/open variants: `secretbox.{Seal,Open}StringWithKey`. **A new sealed column anywhere must be added to `RotateSealedColumns`**, or rotation silently leaves it orphaned under the old key.

---

## Single-Instance Constraint (ADR-0015, v0.3.0)

The 030 migration adds a `runtime_locks` table; [internal/services/runtimelock](internal/services/runtimelock/runtimelock.go) claims the `primary` row at startup, runs a 60s heartbeat, and releases on graceful shutdown. A second container starting against the same SQLite file finds a fresh row and refuses to boot — the error names the foreign hostname/pid + when the row will go stale.

A stale row (5+ minutes without heartbeat — covers `kill -9`'d previous container) is silently overwritten. Operators who don't want to wait the staleness window can run `shellyctl unlock --force` to clear the row manually.

Why: process-local state (rate-limit map, MCP listener, background workers) doesn't replicate across instances. Two containers reading the same DB would double-spawn the firmware-check scheduler, race audit-log retention, and try to re-bind `:8081`. The lock is the explicit door-closer for that misconfiguration.

---

## TOTP 2FA + Personal Access Tokens (Block 4c, v0.3.0)

Two new operator-facing auth surfaces, both built on the existing server-side session store (S5):

**TOTP 2FA (T1)** — operator enrolls a TOTP secret via the Settings UI; subsequent logins require a 6-digit code in addition to the password. RFC 6238 stdlib impl in [internal/services/totp/totp.go](internal/services/totp/totp.go); 10 single-use backup codes issued at enrollment (sha256-hashed + secretbox-sealed in the DB row); wrong code bumps the same per-account lockout counter as wrong-password. [internal/api/handler_totp.go](internal/api/handler_totp.go) drives the `/api/totp/{status,enroll,verify-enroll,disable}` surface.

**Personal Access Tokens (T3)** — bearer-token credentials for headless callers (Home Assistant, cron, scripts) so `/api/*` mutations don't have to fake the cookie + CSRF dance. Token format `pat_<8hex id>_<64hex random>`. Scope catalog (`admin`, `devices:read/write`, `firmware:read/write`, `provision`, `settings:read/write`) gated per-route via `middleware.RequireScope`. Bearer-authed requests skip CSRF (the token IS the proof-of-intent). [internal/services/tokens/tokens.go](internal/services/tokens/tokens.go) is the orchestration; [internal/middleware/auth.go](internal/middleware/auth.go) extends RequireAuth to honor the `Authorization: Bearer pat_…` header. PAT-authed callers cannot mint, list, or revoke other PATs (privilege-escalation guard at the handler).
