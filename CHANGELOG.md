# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [0.1.8] - 2026-05-07 — Per-device action discovery via Shelly.ListMethods

Replaces the hand-rolled five-action surface with a declarative catalog
filtered against each device's `Shelly.ListMethods` output. Adds four new
actions (Wi-Fi scan, Ethernet status read, "reset Wi-Fi & cloud", full
factory reset) and a typed-name confirm modal for the two unrecoverable
ones. See [ADR-0010](docs/adr/0010-per-device-action-discovery-via-listmethods.md)
for the design. Includes migration `021_device_supported_methods.sql`.

### Added
- **Per-device cached method list.** New `supported_methods` column on
  the `devices` row holding the device's `Shelly.ListMethods` output as
  a JSON array. Populated on every firmware-check and refresh; nil
  means "never probed" and the action layer falls back to the v0.1.7
  catalog so the rollout window leaves no device action-less.
- **Declarative action catalog** in `internal/services/actions.go`.
  Each entry declares `RequiredMethods []string` plus an `Apply`
  function; discovery is a filter, not a hand-edited switch.
  `ExecuteDeviceAction` becomes a single dispatch.
- **Four new per-device actions**, gated by their respective RPC
  methods:
  - `wifi_scan` (`Wifi.Scan`) — list visible SSIDs, useful for diagnostics.
  - `eth_status` (`Eth.GetStatus`) — read live link/IPv4/IPv6 status.
  - `factory_reset_wifi` (`Shelly.ResetWiFiConfig`) — clear stored Wi-Fi + cloud config; preserves scripts/KVS/schedule.
  - `factory_reset` (`Shelly.FactoryReset`) — wipe all persisted configuration.
- **Typed-name confirm modal** for `factory_reset` and
  `factory_reset_wifi`. Operator must type the device's name exactly
  before the RPC fires. Reversible high-risk actions (`firmware_update`,
  `reboot`) keep the existing single-click behaviour. ADR-0002 carve-out
  documented in the plan.
- **Risk-grouped action ordering** — the API now returns actions
  sorted low → medium → high so the front-end renders a natural
  click-freely → confirm-required progression.
- New tests: `methodsCovered` branches, fallback-when-unprobed,
  filter-by-methods, online-gate, risk ordering, dispatch table.

### Changed
- `BLE.Pair` action's runtime "skipped on unsupported firmware" path
  now functions as a fallback: once `SupportedMethods` is populated the
  catalog filter handles it before the RPC fires. Old behaviour kept
  for the rollout window.
- `app_clients.refreshFirmwareCache` renamed to
  `refreshDeviceCapabilities` and extended to also pull
  `Shelly.ListMethods`. Three RPCs per online device per refresh
  (CheckForUpdate + ReadAutoUpdate + ListMethods); negligible
  wall-time at concurrency-64 fleet sizes.

### Migration notes
- Migration `021_device_supported_methods.sql` adds a single
  `supported_methods TEXT NOT NULL DEFAULT ''` column. Empty string =
  "never probed", populated by the next firmware-check or refresh on
  each device.

## [0.1.7] - 2026-05-06

Scheduled firmware checks, configurable install timeout, the legacy
firmware/credential column drop, and a CI auto-release pipeline. No
operator-visible change to existing workflows; the new surfaces are
opt-in via Settings → Firmware. Includes migrations
`019_drop_legacy_fw_columns.sql` and
`020_drop_legacy_credential_columns.sql`.

### Added
- **Scheduled firmware checks** — new `firmware_check_interval` setting
  (seconds, 0 = disabled). A long-lived background goroutine polls
  AppSettings every 60 s, fires `StartFirmwareCheck` at the configured
  cadence, and skips ticks when a manual check is already running.
  Settings UI exposes presets: Off / Hourly / 6h / 12h / Daily / Weekly.
- **Configurable per-device install timeout** — new
  `firmware_install_timeout` setting (seconds, default 300). Replaces
  the previous hardcoded 5 min. Per-device, not job-total. Surfaced in
  the timeout detail line ("device still on X after 8 min" etc.).
- **Auto-release on tag push** —
  `.github/workflows/publish-image.yml` now extracts the matching
  CHANGELOG entry via awk and calls `gh release create` (or
  `gh release edit` if the release was hand-created), so future `v*`
  tag pushes produce a GitHub Release alongside the GHCR image. Tags
  matching `*-rc*` / `*-beta*` / `*-alpha*` are auto-marked as
  prereleases.
- Unit tests for `firmwareInstallTimeoutFromSettings`,
  `firmwareSchedulerDecision`, and `formatTimeout` covering all
  reachable branches (`internal/services/app_jobs_test.go`).

### Changed
- **Migration `019_drop_legacy_fw_columns.sql`** drops `fw_status` and
  `fw_available_ver` from `devices`. Both columns have been
  unread/unwritten by Go code since v0.1.5; the rollback window is
  closed.
- **Migration `020_drop_legacy_credential_columns.sql`** drops
  `password` and `ha1` from both `credentials` and `credential_groups`.
  These columns have been zeroed at every boot since v0.0.15 (the
  one-shot encryption sweep). Cipher columns
  (`password_cipher`, `ha1_cipher`) are now the only at-rest source.
  The `encryptPlaintextCredentials` sweep is removed; `resolveSecret`
  is replaced by a trivial `decryptCipher` helper.
- `.gitignore` now excludes `.claude/` so the local CLI working
  directory stops appearing in `git status`.

### Migration notes
- SQLite `ALTER TABLE DROP COLUMN` is in-place but does not VACUUM the
  file. Plaintext bytes from pre-v0.0.15 installs may remain on disk
  pages until SQLite recycles them. Operators with strict scrubbing
  requirements should run
  `sqlite3 ${DATA_DIR}/shellyctl.db "VACUUM"` once after upgrade.
- Downgrade below v0.1.7 is not supported on installs that have run
  migrations 019/020. The dropped columns can't be recovered without a
  pre-upgrade backup.

## [0.1.6] - 2026-05-06

Adds firmware **auto-update** support via the device's `Schedule.*` API (the
same mechanism the device's own web UI uses) and surfaces it across the
Firmware, Devices, Compliance, and Provision pages. Refresh is extended to
also sync firmware availability + auto-update mode, so it's now the single
"make this row reflect reality" button. Includes migration
`018_device_fw_auto_update.sql`. CI: migrated to golangci-lint v2 +
`golangci-lint-action@v9` ahead of the 2026-06-02 Node 20 sunset.

### Added
- **`fw_auto_update` per-device** (`""` | `off` | `stable` | `beta`).
  Shelly Gen2+ exposes no dedicated OTA-config method; the local web UI
  implements its "Enable auto update firmware" toggle (FW 1.2.0+) as a
  `Schedule.Create` job that calls `Shelly.Update` with
  `origin: "shelly_service"`. ShellyAdmin honours the same marker so
  user-created Schedule entries are not clobbered. New helper module
  `internal/core/firmware/autoupdate.go`.
- **Bulk auto-update buttons** on the Firmware page: `Auto → Off / Stable
  / Beta` operate on the row selection. Row checkboxes are now
  channel-agnostic (no longer disabled when a device is already on the
  latest firmware), so any device can be a target. `Update N` still
  filters internally to install-eligible rows.
- **`set_auto_update` bulk action** in the device-action API
  (`POST /api/bulk` with `value: "off"|"stable"|"beta"`).
- **Auto-Update column** on both the Firmware and Devices pages
  (toggleable on Devices via Columns).
- **Compliance rule** `auto_update_stage` in its own SectionCard
  "Auto-Update Schedule (Gen 2+, FW 1.2.0+)". Devices whose schedule
  doesn't match flag as non-compliant. Skipped on devices not yet
  firmware-checked so mixed fleets don't false-positive.
- **Provision template section** `auto_update`. Canonical bare-string
  encoding (`"auto_update": "stable"`); also accepts
  `{stage: "stable"}`. New "Auto-Update Schedule (Gen 2+)" section in
  the Provision Misc form. Backend handler in `applySection`.

### Changed
- **Refresh now syncs firmware data.** Per-device Refresh (Devices page)
  and bulk Refresh both call `Shelly.CheckForUpdate` + `Schedule.List`
  per online device, updating `fw`, `fw_available_stable`,
  `fw_available_beta`, `fw_checked_at`, `fw_auto_update` in the same
  pass. Refresh stops being a "data subset" relative to Check Firmware.
- **Refresh no longer blanks the firmware cache.** Latent regression:
  `scanner.ProbeDeviceWithOptions` constructed a fresh Device that
  zeroed the firmware fields. The fix carries the persisted values
  forward before the firmware re-check writes fresh ones, so a
  transient cloud blip during refresh leaves your cache intact.
- **Compliance: Auto-Update rule lives in its own SectionCard** between
  Zigbee and "FW 2.0+ checks". Previously nested inside the 2.0+
  section, which misled operators since Schedule.* works on FW 1.2.0+.
- **CI: golangci-lint v1.64 → v2.6**; `.golangci.yml` migrated via
  `golangci-lint config migrate`; action `@v6` → `@v9` (Node.js 24).
- **Bundle budget** bumped to 280 kB raw / 80 kB gzip to absorb the
  v0.1.5+v0.1.6 surface additions.

### Fixed
- Row checkboxes on the Firmware page no longer auto-uncheck themselves
  on channel toggle — devices on the latest firmware can now be
  selected for the auto-update bulk actions.
- Bulk auto-update status message moved out of the toolbar into a
  dismissable inline notice between toolbar and progress bars.

### Migration notes
- Migration `018_device_fw_auto_update.sql` adds the `fw_auto_update`
  column. Empty default = "never read"; populated by the next firmware
  check or refresh on each device.

## [0.1.5] - 2026-05-06

Full rebuild of the firmware update page: dual-channel availability
cache, dedicated install-progress job, and confirmation modal — driven
by a `/grill-me` design pass after multiple bug reports against the
v0.1.4 page. Adds migration `017_device_fw_per_channel.sql`.

### Added
- **Per-device, per-channel firmware cache** on the Device row:
  `fw_available_stable`, `fw_available_beta`, `fw_checked_at`.
  `Shelly.CheckForUpdate` returns both stable and beta sections in a
  single response; we now persist both. The Firmware page channel
  selector becomes a pure display + install filter — toggling is
  instant with no re-check.
- **Dedicated `firmware_install` job** replacing the fire-and-forget
  `Shelly.Update` path. Bounded concurrency (5 in flight), per-device
  polling of `Shelly.GetDeviceInfo` every 5 s until version match,
  hard 5-min timeout per device. New `GET /api/firmware/install/status`
  surfaces live progress.
- **Confirmation modal** on bulk update — lists affected device names,
  IPs, and target version, plus the channel, before any RPC fires.
- **Sortable Firmware table.** Click any column header (Name, Gen,
  Model, IP, Current, Available Stable, Available Beta, Status). IP
  sorts numerically by octet.
- **Select-all checkbox** in the table header with indeterminate state
  when only some rows match.
- **Configurable Gen 2 / 3 / 4 badge colors** (Settings → UI
  Preferences). Seven-preset Bootstrap palette with live preview.
- **Shared Stable/Beta channel store** (persisted to localStorage) so
  toggling on the Firmware page also takes effect on the Devices page
  and vice versa.
- **Out-of-band firmware drift detection.** Every firmware check now
  also calls `Shelly.GetDeviceInfo` and writes the running version
  back to `Device.FW`, so devices upgraded via the device's own web UI
  reflect their current firmware in ShellyAdmin without needing a
  separate Refresh.
- **Firmware columns on the Devices page** (`fw_available_stable`,
  `fw_available_beta`) so update availability is visible from the
  primary list, not just the Firmware page.

### Changed
- **`firmware.Result` rebuilt to dual-channel**: `stable_ver`,
  `beta_ver`, `stable_update`, `beta_update`, `status`, `note`,
  `checked_at`. Old `update_available` / `available_ver` / `stage`
  fields removed. `pickStage` / `stageNote` helpers deleted — they
  silently fell back to the other channel and caused wrong-channel
  ghost updates.
- **Friendlier RPC errors.** Timeouts → "device did not respond in
  time", connection failures → "connection refused" / "no route to
  host", DNS failures → "DNS lookup failed". Anything unrecognized is
  truncated to 120 chars instead of dumping a raw Go stack into the
  status detail line.
- **Firmware install timeout message** now describes what actually
  failed: "device still on 1.7.5 after 5 min (expected 1.8.99)"
  instead of the previous "did not come back in time".
- **Per-device firmware check timeout** bumped 5 s → 10 s.
- **Stale install overlay clears on the next firmware check**, so a
  fresh check meaningfully resets the page.

### Fixed
- **`selected` Set reactivity** — Svelte 4 doesn't track `.add()` /
  `.delete()` mutations, so the "Update N" counter never updated
  ([previous behaviour: counter stayed at 0]). Replaced the Set with
  `bind:group` against a `string[]`. Also fixes the "counter shows 1
  but no row checked" stale-MAC bug, and the "still selected after
  channel toggle" bug.
- **Wrong-channel ghost updates** when stable was selected but only
  beta had an update — `pickStage` would silently fall through, the
  row got marked updateable, then `Shelly.Update` was called with
  `stage: stable` and silently no-op'd.
- **Status badge stuck on "update" forever** after a successful
  install — now flips to "current" automatically once the device
  reboots onto the new firmware.
- **Missing name + model** on the Firmware page rows; **non-clickable
  IP**. Both addressed in the new column layout (clickable IP opens
  the Shelly device's own web UI in a new tab).

### Migration notes
- Migration `017_device_fw_per_channel.sql` adds three columns
  (`fw_available_stable`, `fw_available_beta`, `fw_checked_at`).
  Existing rows get empty defaults until the next firmware check
  populates them. Legacy `fw_status` / `fw_available_ver` columns are
  left in place but no longer read or written.

## [0.1.4] - 2026-05-04

UX: remove the section-level "enable section" checkbox from every form on
the Provision page. Now matches the Compliance page behaviour — sections
auto-expand whenever any inner field toggle is on. Fixes the long-standing
confusion where ticking the section header then ticking inner toggles made
the section feel impossible to collapse.

### Changed
- **`SectionCard` `enabled` prop is no longer passed by Provision forms**
  (Sys, Mqtt, Ws, Ble, Matter, Cloud, Auth, Wifi (and sta/sta1/roam),
  WifiAP, Modbus, Zigbee, Eth, UI, Scripts). The `enabled: boolean` field
  was removed from each State type, and the `if (!s.enabled) return null`
  early-return in every `build*` function was dropped — sections are
  emitted whenever they have at least one inner field set, exactly the
  same logic that already gated each individual field.
- **Inner-field `disabled={!state.enabled || ...}` guards removed.** Each
  inner FieldRow / Toggle / input now disables purely on its own
  `*Enabled` flag.
- **Hydration no longer sets `state.enabled = true`** when a saved
  template loads — the inner fields determine visibility.

### Operational note
Saved templates continue to load correctly. The on-wire JSON shape is
identical to v0.1.3 for sections that have ≥ 1 field set; sections that
were "enabled but empty" in v0.1.3 (which sent `{}` to the device, a no-op
in practice) are now omitted entirely.

## [0.1.3] - 2026-05-04

Third patch fix for the v0.1.0 scanner false-positive issue. v0.1.1
caught empty bodies, v0.1.2 caught Basic-auth 401s, and v0.1.3 catches
the **HTTP→HTTPS redirect to a self-signed cert** path used by UniFi
UDM Pro Max — the device redirects HTTP `/shelly` to HTTPS, the TLS
handshake fails on the self-signed cert, and ShellyAdmin was converting
the resulting `ErrTLSCertInvalid` into a partial Device record. This
patch generalises the fix so any recoverable probe failure on an
unknown IP is dropped, not persisted.

### Fixed
- **Probe failures during scan no longer create partial Device records.**
  `scanner.ProbeOptions` gained a `KnownMAC` field. When set (refresh
  path), recoverable failures (auth-required, lockout, TLS-cert-invalid)
  produce a partial record carrying that MAC so the existing device row
  stays accurate. When empty (scan-of-unknown-IP path), recoverable
  failures yield `nil` — without positive Shelly evidence we have
  nothing to persist (`internal/core/scanner/scanner.go`,
  `internal/services/app_clients.go`).

### Tests
Regression coverage for: HTTP→HTTPS redirect to a self-signed cert
(UDM Pro Max shape), Basic-auth 401 at the scanner layer (both with
empty `KnownMAC` should yield nil; with non-empty `KnownMAC` should
yield a partial record).

## [0.1.2] - 2026-05-03

Second patch fix for the v0.1.0 scanner false-positive issue: v0.1.1
caught the empty-body and non-Shelly-JSON cases but missed the most
common UniFi case — UDM Pro / Protect cameras return `401 Unauthorized`
with `WWW-Authenticate: Basic realm="..."` on `/shelly`. v0.1.1's auth
handler treated *any* 401 as "Shelly with creds needed" and returned
`ErrAuthRequired`, which the scanner converted into a partial Device
record with empty MAC. Even though the MAC primary key dedupes them at
write time, they still flashed in the live scan-progress UI.

### Fixed
- **Non-Digest 401 responses no longer surface as `ErrAuthRequired`.**
  A real Shelly always uses RFC 7616 Digest auth; a 401 with `Basic`,
  `Bearer`, or no `WWW-Authenticate` challenge means the endpoint isn't
  Shelly. `shellyclient.do` now returns a generic descriptive error in
  that case, which `reportProbeFailure` ignores — no partial record
  created, IP skipped cleanly (`internal/core/shellyclient/client.go`).

### Other fixes
- **Power-readings voltage was order-dependent (flake).** The
  `extractPowerReadings` function clobbered `lastV` for switch / pm1 /
  em1 components but took max for em (3-phase). Go map iteration order
  is randomized, so `TestExtractPowerReadings_SwitchAndEM` would fail
  ~50 % of the time. Logic now consistently takes the maximum non-zero
  voltage across all components (`internal/core/scanner/scanner.go`).
- Test failure messages now dereference `*float64` pointers so the
  output reads "VoltageV = 230, want 232" instead of a hex address.

## [0.1.1] - 2026-05-03

Patch fix for a v0.1.0 scanner regression: non-Shelly endpoints (UniFi
UDM Pro / Protect cameras, generic web servers, captive portals) that
answered `GET /shelly` with `200 OK` and an empty / non-Shelly JSON body
were being persisted as junk Device records with empty MAC and an
arbitrary Gen=2 default.

### Fixed
- **Scanner now rejects non-Shelly responses on `/shelly`.** A real
  Shelly always reports either a non-empty `mac` or a non-zero `gen`
  field; without one of those the IP is logged at DEBUG and skipped
  (`internal/core/scanner/scanner.go`, `internal/core/shellyclient/client.go`).
- `shellyclient.Probe` now treats an empty 200 body as an error rather
  than returning an empty map. The pre-v0.1.0 code did this implicitly
  via `json.Decoder.Decode` returning `io.EOF`; the v0.1.0 rewrite to
  `io.ReadAll` + `json.Unmarshal` lost that guard.

### Operational note
Existing junk Device rows (created by v0.1.0 scans before this fix) need
to be cleaned up manually via the Devices page row-level remove button —
the migration does not retroactively delete them. A subsequent scan on
v0.1.1 will not re-create them.

## [0.1.0] - 2026-05-03

Adapt ShellyAdmin to Shelly Gen2+ firmware **2.0.0-beta1**. The release adds an
RFC 7616 Digest auth client, HTTPS scheme handling with per-device TLS policy,
strips the removed BLE `enable` flag, surfaces new compliance fields
(enhanced_security, tls_cert_valid, wifi_hostname), and exposes the per-device
WiFi hostname in the provisioner UI. Includes one schema migration
(`015_device_fw2_fields.sql`).

### Added
- **`internal/core/shellyclient`** — unified HTTP/JSON-RPC client used by every
  device-talking code path (scanner, provisioner, setters, firmware). Implements
  RFC 7616 Digest auth (SHA-256 with MD5 fallback, qop=auth, nonce-counter
  reuse), 429 brute-force-lockout signalling via `ErrAuthLockout`, and
  configurable TLS policy. Old call sites kept their timeout-only signatures via
  back-compat wrappers; new `*WithOptions` variants thread credentials and
  scheme through per device.
- **Device fields** for FW 2.0 state: `Scheme`, `EnhancedSecurity`,
  `TLSCertValid`, `TLSAllowInsecure`, `AuthLockedUntil`, `WiFiHostname`,
  `WiFiChannel`. Migration `015_device_fw2_fields.sql` adds the columns;
  existing rows get `scheme="http"` and null TLS/EnhancedSecurity.
- **Compliance rules**: `enhanced_security`, `tls_cert_valid`, `wifi_hostname`,
  `ble_paired`, `webhooks_configured`. Mixed-fleet safe — rules are skipped
  when the device hasn't reported the underlying state.
- **Per-device credential lookup** for refresh/firmware/setter paths via the
  existing credential-group → credential pipeline. Bulk actions now run with
  the device's assigned credential automatically.
- **WiFi hostname** field in the provisioner STA form, hydrated from saved
  templates. Routes through `Wifi.SetConfig`'s native `sta.hostname`.
- **Cover provisioner section** (`case "cover"`) with normalizer hook for the
  slat/tilt config introduced for venetian-blinds support.
- **Cover.GoToTilt setter** for slatted-cover bulk control.
- **Webhooks provisioner section** (`case "webhooks"`) — declarative
  delete_all → delete → update → create pipeline driving Webhook.* RPCs.
  Method-not-found errors on older firmware surface as "skipped" so mixed-fleet
  templates don't blow up.
- **LNM provisioner section** (`case "lnm"`) — explicit handler so the
  all-caps `LNM.SetConfig` method routes correctly (the catch-all would
  produce `Lnm.SetConfig`).
- **BLE pair device action** — new per-device action `ble_pair` that calls
  `BLE.Pair`. Surfaces "skipped" on firmware that doesn't expose the RPC.
- **Live power telemetry** (Phase C1+C4): scanner extracts `apower`,
  `voltage`, `current` from switch/em/em1/pm1 status components and sums them
  into device fields `PowerW`, `VoltageV`, `CurrentA`. Surfaced on the
  Devices list (sortable columns) and a "Live Readings" card on DeviceDetail.
- **Compliance UI** for the firmware 2.0 fields — new SectionCard with toggles
  for `enhanced_security`, `tls_cert_valid`, `wifi_hostname`, `ble_paired`,
  `webhooks_configured`. Mixed-fleet safe; rules are skipped when the
  underlying state isn't reported.
- **Migration `016_device_power_readings.sql`** adds `power_w`, `voltage_v`,
  `current_a` columns.

### Changed
- **BLE `enable` flag stripped at provisioning time** with a per-device warning,
  matching FW 2.0.0-beta1 which removed the flag (BLE auto-activates with
  scanning). The toggle is removed from the provisioner BLE form; older
  templates that still set `ble.enable` continue to load with a console warning.
- **Provisioner / setters / firmware / scanner** route every RPC through
  `shellyclient.Client` instead of bare `http.Client`. Method-not-found is now
  detected via the typed `RPCError` rather than ad-hoc body parsing.
- **Refresh path** distinguishes recoverable failures: `AuthRequired`,
  `AuthLockedUntil`, and `TLSCertValid=false` partial probes update the
  device row without flipping it offline.

### Security
- Authenticated probing means devices behind admin auth (FW 1.x and 2.0+)
  no longer silently appear offline; we now actually authenticate when a
  credential is mapped to the device.
- HTTPS scheme awareness: when a 2.0 device redirects HTTP→HTTPS (with
  `enhanced_security` enabled), the scheme is remembered and reused on
  subsequent calls. Per-device `tls_allow_insecure` opt-out for self-signed
  certs.

## [0.0.16] - 2026-04-24

Follow-up release after a combined `/review` and `/security-review` of the
v0.0.7 → v0.0.15 window. Closes one medium-severity provisioning leak, finishes
the v0.0.14 OTA removal across the UI, surfaces bulk-refresh auth failures,
redirects the SPA on expired sessions, plumbs request IDs into service-layer
logs, tightens settings validation, and dedupes helpers the earlier passes left
behind. No schema migrations.

### Security
- Remove undocumented `${ENV:...}` env-var expansion from provisioning template
  substitution (`internal/core/provisioner/provisioner.go`). The feature allowed
  an authenticated admin to exfiltrate server env vars (including
  `SHELLYADMIN_PASS_HASH`, `SHELLYADMIN_SECRET`, `SHELLYADMIN_ENCRYPTION_KEY`)
  by POSTing a crafted template to an attacker-controlled LAN IP. Only the
  documented `{device_name}` token remains.

### Fixed
- **Bulk refresh now surfaces `AuthRequired` / `AuthError`** the same way the
  single-device path does, so a password-mismatch device no longer shows a
  generic "refresh timed out" row (`internal/services/app_jobs.go`).
- **`debug.mqtt` passthrough in provisioning templates** — the toggle existed
  in `SysForm.svelte` but the backend normaliser dropped it; it is now
  preserved alongside `debug.websocket` and `debug.udp`
  (`internal/core/provisioner/provisioner.go`).
- **SPA redirects to `/login` on 401** for expired sessions instead of showing
  an opaque error (`web/src/lib/api.ts`).
- **`session.Save()` errors are no longer swallowed** on login or logout; login
  now returns 500 on persistence failure instead of handing the user a broken
  cookie (`internal/api/handler.go`).
- **Compliance custom-rule regex is validated at save time**; a bad pattern no
  longer silently classifies every device as non-compliant
  (`internal/services/app.go`, `internal/core/compliance/compliance.go`).
- **Lat/Lon bounds (`±90` / `±180`) enforced in `ValidateSettings`** so invalid
  compliance settings are rejected on save, not only when a bulk action runs
  against them (`internal/services/app.go`).

### Changed
- **OTA removal finished across the UI and backend** (started in v0.0.14).
  Frontend: dropped the Provision → Misc "OTA" section, the `OtaState` type,
  and `ota_auto_update` from the Compliance page. Backend: dropped
  `OTAAutoUpdate` from `models.AppSettings` / `ComplianceRules`, the OTA branch
  in `applySection`, and the `normalizeOTAPayload` helper. The `ota` key is
  still accepted as a passthrough via the default handler (it 404s on the
  device and is skipped gracefully) to avoid breaking anyone with existing
  template JSON.
- **Request IDs propagate into service-layer logs.** The service log callback
  now takes `context.Context`; request-scoped sites (Provision, UploadUserCA,
  BulkAction, ExecuteDeviceAction, RefreshDevices, Stop) log via a new
  `LogCtx` helper so the ID populates both the `audit_log.request_id` column
  and the slog JSON attribute. Background jobs (scan, firmware, recovery)
  intentionally keep the no-ctx path since they outlive the HTTP request
  (`internal/services/app.go`, `internal/services/app_jobs.go`,
  `internal/services/device_surface.go`, `internal/api/handler.go`,
  `cmd/shellyctl/main.go`).
- **`AppService.Stop` is idempotent** via `sync.Once` so overlapping signal
  handlers can't re-cancel or re-mark interrupted jobs.
- **Shared `sslCAOptions` dropdown** for the Provision MQTT and WebSocket
  forms. `WsForm` previously took a freeform input; both now use the same
  four-option Select (`""`, `*`, `ca.pem`, `user_ca.pem`) matching the only
  values the Shelly API accepts (`web/src/pages/provision/sslCa.ts`).
- **`firstNonEmpty` deduped** into a single exported `util.FirstNonEmpty`
  (trim-variant) used by the provisioner, service, and user-CA code paths
  (`internal/util/strings.go`). Removed the dead `boundedConcurrency` alias
  and the dead `gen int` parameter on every setter in `internal/core/setters`.

## [0.0.15] - 2026-04-22

Security-hardening round: argon2id admin password hashing, encryption at rest for
device credentials, request correlation IDs across the audit trail, sanitized HTTP
error responses, and Svelte lint re-enabled in CI. No user-facing workflow changes;
all migrations are forward-only and transparent on first boot.

### Added
- **Argon2id admin password hashing** via a new `SHELLYADMIN_PASS_HASH` env var
  (also honours `_FILE`). Generate the PHC string with `shellyctl hash-password
  <plaintext>` (a new subcommand on the same binary, also runnable via
  `docker run --rm ghcr.io/buliwyf42/shellyadmin:latest shellyctl hash-password …`).
  Plaintext `SHELLYADMIN_PASS` still works for backward compatibility but emits a
  deprecation warning on startup; removal planned for a future release.
- **Encryption at rest for device credentials** (`credentials` and
  `credential_groups` `password` / `ha1` columns). XSalsa20-Poly1305 via
  `nacl/secretbox` with a 32-byte key resolved from `SHELLYADMIN_ENCRYPTION_KEY`
  (base64, `_FILE` suffix supported) or generated on first boot at
  `${DATA_DIR}/shellyadmin.key` (0600). Migration 013 adds cipher columns and a
  one-shot sweep on startup rewrites any legacy plaintext rows. **Back the key
  file up alongside the database** — losing it permanently orphans every stored
  credential.
- **Request correlation IDs** (`X-Request-ID`) generated by a new
  `internal/middleware/requestid` middleware: 16-hex per request, echoed on every
  response, honours client-supplied IDs (alnum/`-`/`_`, truncated to 64 chars).
  Stashed on both gin and stdlib contexts. Audit entries persist the ID in a new
  `audit_log.request_id` column (migration 014); the Logs page surfaces it and CSV
  export includes the new field.
- **Structured slog mirror** for audit lines. Operator-tailing the container log
  now sees JSON records with `request_id` attributes alongside the SQLite-backed
  audit trail.
- **Scanner network allowlist + tighter CIDR cap** (`internal/core/scanner`):
  targets must be RFC1918 or link-local, loopback/multicast/public ranges are
  rejected, and max CIDR size drops from `/16` (65K hosts) to `/22` (1024 hosts).
- **`Store` interface at the service/DB boundary** (`internal/services/store.go`),
  enabling unit tests against a fake persistence layer without spinning up SQLite.
- **Per-device bulk-action audit detail**: `summarizeBulkResults` now records
  ok / failed / skipped / missing counts and MAC lists (truncated at 20 + overflow)
  so a "what did this action touch?" question is answerable from the audit log alone.
- **CI lint and format gates** — `.golangci.yml` (govet, staticcheck, errcheck,
  ineffassign, unused, gofmt, goimports, misspell, unconvert) wired into the
  backend job; `web/eslint.config.js` (TS + `eslint-plugin-svelte`), `.prettierrc.json`
  (with `prettier-plugin-svelte`), and new `lint` / `format:check` scripts wired
  into the frontend job.
- **New documentation**: `docs/roadmap.md` as the source of truth for planned
  direction, linked from the README and the ADR index.

### Changed
- **Handler error responses are now sanitized**. Internal error details (stack
  traces, filesystem paths, DB quirks) are logged in full via the request-scoped
  audit path but never echoed to the client — 5xx responses return a generic
  `{"error": "<publicMsg>"}` body. User-facing validation errors still echo through
  via `respondUserError` so operator guidance (e.g. `scan_timeout must be
  positive`) is preserved.
- **Audit logging contract** now flows through a single `Handler.auditSink`
  pluggable for tests, replacing the previous scattered `logFn` method. Request
  ID is injected at the sink layer, so every `respondError` / `respondUserError`
  call automatically carries the correlation ID.
- **CSV log export** column order: `id, ts, level, request_id, message` (was
  `id, ts, level, message`).

### Security
- Admin password at rest is now a salted, memory-hard hash rather than plaintext
  when `SHELLYADMIN_PASS_HASH` is configured.
- Device credentials stored in SQLite are no longer readable from an offline DB
  copy (container escape, stolen backup, misconfigured volume). Does not defend
  against attackers with live process memory access.
- HTTP error responses no longer leak backend implementation details to
  authenticated callers.
- Scanning is constrained to local networks the operator is meant to be managing;
  accidental `0.0.0.0/0` or similar inputs are rejected.

### Migration notes
- Existing deployments will have `${DATA_DIR}/shellyadmin.key` generated on first
  boot under v0.0.15. Add this file to your backup rotation.
- Plaintext admin password continues to work; switch to the hash path at your own
  pace using `shellyctl hash-password`.
- Old legacy plaintext columns on `credentials` / `credential_groups` are retained
  for one release to keep a safe rollback window; they will be dropped in a
  follow-up migration.

## [0.0.14] - 2026-04-21

Bug fixes for the v0.0.13 UI improvements.

### Fixed
- **ProgressBar idle state** — bar now renders empty at 0/0 instead of showing a full-width gold fill (the fill div previously had no explicit width when `total=0`).
- **ProgressBar label split** — the "label below" span was inside `.pb-track` (which has `overflow:hidden`), causing the track to expand vertically and the fill to cover only the upper half. Span moved outside the track div.
- **OTA compliance rule always firing "unsupported"** — `OTA.SetConfig` does not exist on Shelly Gen2 devices so `ota.auto_update` is never present in any device config; the compliance check therefore always returned "unsupported" regardless of the configured rule. Removed the evaluation from the backend (`compareConfigStringOrUnsupported` helper and `OTAAutoUpdate` call), removed the OTA section from the Compliance settings UI, and cleared any previously saved value on next settings save.
- **OTA form radio buttons** — the "Update automatically" field in the Provision form and (previously) Compliance page used a `<Select>` dropdown; replaced with radio buttons matching the Shelly web interface (Disable / Enable stable / Enable beta, with an italic beta-instability warning).

## [0.0.13] - 2026-04-21

Completes the WiFi / provisioning / UI improvement sprint: full Wifi.SetConfig surface coverage (STA1, roaming, static IPv4), new Script / UI / Eth-IPv6 provisioner sections, per-device restart_required feedback after provisioning, reboot controls on the Devices page, and a shared polished progress bar component.

### Added
- **WiFi STA1 + roaming + static IPv4** provisioner coverage. The Provision form now exposes the full `Wifi.SetConfig` surface: a second station (`sta1`), roaming config (`rssi_thr`, `interval`), and per-STA static IPv4 fields (`ip`, `netmask`, `gw`, `nameserver`). New `WifiStaForm.svelte` and `WifiRoamForm.svelte` components reuse the existing field-enable-checkbox pattern.
- **Script, UI, and Eth-IPv6 provisioner sections**. `Script.SetConfig` per-id loop (mirrors the existing KVS handler), `UI.SetConfig` fields (`idle_brightness`, `debug_enable`), and Eth IPv6/DNS fields (`ipv6mode`, `ipv6` block). New `ScriptsForm.svelte` and `UIConfigForm.svelte` Provision-page forms; `EthForm.svelte` extended with a collapsible IPv6 section.
- **`restart_required` surfaced after provisioning**. Every `*.SetConfig` RPC response includes a `restart_required` flag. `SectionResult` now carries this field; `rpcSection()` parses it; the provision API response includes a device-level `restart_required` boolean. The Provision results view shows a gold **"restart required"** badge per device and a "Reboot N restart-required devices" button.
- **Reboot controls on the Devices page**. Per-row **⏻ reboot button** (confirm dialog, inline spinner, result notice) and a **Reboot All** toolbar button that reboots all currently-listed devices with a count-aware confirm. Both hit `POST /api/bulk` with `action: "reboot"`.
- **Bulk `reboot` action** wired into the backend (`validateBulkAction`, `applyBulkAction` → `setters.Reboot`, `bulkActionSummary`, `bulkActionWarnings`).
- **Shared `ProgressBar.svelte` component** (`web/src/components/`). Determinate mode: gold gradient fill with `width: 200ms` transition and animated 45° stripe overlay while running. Indeterminate mode (total = 0 while running): full-width stripe animation. Solid on completion, empty when idle at 0/0. Proper `aria-valuenow/min/max` when determinate, `aria-busy` while running, `aria-label` prop. Label inside fill when ≥ 25% wide, below otherwise.

### Changed
- Firmware and Scan pages now use `ProgressBar.svelte` instead of duplicated inline `<div class="progress">` markup. The unused `progress-bar-striped` class is gone.

### Fixed
- `SectionResult.RestartRequired` propagates correctly even when the section status is `ok` (previously the field was parsed but discarded before the return).

## [0.0.12] - 2026-04-21

Closes several Shelly API coverage gaps identified in the 2026-04 review: new compliance rules and provisioning surfaces for previously-unexposed device subsystems, plus chunked certificate upload that unblocks MQTT/WS with a user-managed CA and mTLS-to-broker auth.

### Added
- **User CA + TLS client cert/key upload** via chunked `Shelly.PutUserCA` / `Shelly.PutTLSClientCert` / `Shelly.PutTLSClientKey` RPCs. New `POST /api/provision/user-ca` endpoint (optional `kind` field: `user_ca` | `tls_client_cert` | `tls_client_key`) and a Provision-page **Upload Certificate (PEM)** form with a kind selector. Closes the MQTT/WS `user_ca.pem` loop and enables mTLS-to-broker authentication.
- **Per-IP concurrency guard** on certificate uploads — reuses the existing Provision/Firmware reservation pattern; uploads that collide with an in-flight Provision or Firmware job on the same device return a `skipped` result with a `device busy` detail instead of silently racing.
- **New compliance rules**: `wifi_ap_enabled`, `wifi_ap_is_open`, `eth_enabled`, `eth_ipv4mode` (`dhcp` | `static`), `sys_debug_mqtt`, `matter_enabled`, `modbus_enabled`, `zigbee_enabled`. Each rule surfaces in the Compliance page with the same toggle + enable-checkbox pattern as the existing rules.
- **New provisioner sections + UI forms**: `eth` (via `Eth.SetConfig`) and UI-only `wifi_ap`, `modbus`, `zigbee`, `user_ca` forms wired into the Provision page. The `eth` section joins the existing `mqtt`/`ws`/`wifi`/… section handlers in `applySection()`.
- Service-layer test suite `internal/services/user_ca_test.go` covering all input-validation paths (empty/too-many IPs, unknown kind, empty/headerless/oversized PEM, invalid/non-local IP) and a busy-target concurrency-guard case.
- Parameterized provisioner tests in `internal/core/provisioner/user_ca_test.go` exercising the chunked-upload sequence and back-compat wrapper across all three certificate kinds.

### Changed
- `internal/core/provisioner/user_ca.go` generalized around a `CertificateKind` enum (`KindUserCA` / `KindTLSClientCert` / `KindTLSClientKey`) with shared `UploadCertificate` / `RemoveCertificate` helpers. The original `UploadUserCA` / `RemoveUserCA` entry points are preserved as thin back-compat wrappers.
- `compliance.Evaluate` now honours the new rules via the existing `compareConfigBool` / `compareConfigString` helpers (no behaviour change for unset rules).
- `ComplianceRules.Normalize` coerces `eth_ipv4mode` to `dhcp` / `static` / empty — anything else is dropped rather than applied literally.

## [0.0.11] - 2026-04-18

Provision and Compliance UI refresh: dated `<select>`-based On/Off controls replaced with real toggle switches and a styled custom dropdown; section cards, field rows, and the Provision toolbar cleaned up. Plus a long-standing template-load bug fix.

### Added
- Four reusable form primitives under `web/src/components/`: `Toggle.svelte` (switch), `Select.svelte` (keyboard-navigable custom dropdown), `FieldRow.svelte` (enable-checkbox + label + control), and `SectionCard.svelte` (collapsible card with optional enable checkbox in the header). All are token-backed and reuse existing CSS variables (`--panel-2`, `--border`, `--warning`, `--radius-md`, `--control-height`) — no new dependencies.
- New token-backed component styles in `web/src/app.css` (`.sa-section`, `.sa-toggle`, `.sa-select`, `.sa-field`, `.sa-check`, `.sa-form-grid`, `.provision-toolbar`, `.sa-cluster`, `.sa-view-switch`).

### Changed
- Bulk actions `set_cloud_enabled` and `set_ble_enabled` (POST `/api/bulk`) toggle `Cloud.SetConfig {enable}` and `BLE.SetConfig {enable}` on the selected devices. Same preview / dry-run / per-target eligibility behavior as the existing toggles.
- Test coverage: `internal/core/setters` now has an `httptest`-backed unit test per setter (Sys, MQTT, Cloud, BLE, Reboot); `internal/db` has tests for `UpsertDevices` atomic commit, the two-miss offline transition, and error surfacing on a closed DB; `web/src/components/sortHeader.test.ts` covers the sort-direction derivation.
- `web/src/components/SortHeader.svelte` now derives its aria/indicator state from a small `sortHeader.ts` helper instead of inlining the logic — same behavior, but the derivation is unit-tested.
- Provision sub-forms (`SysForm`, `MqttForm`, `WsForm`, `BleForm`, `MiscForm`) and `Compliance.svelte` migrated to the new primitives. All On/Off `<select>` blocks replaced by Toggle; multi-value dropdowns (TLS mode, OTA stage, auto-update policy, custom-rule source/op) replaced by the custom Select; repeated "enable checkbox + label + control" markup now flows through FieldRow.
- Provision toolbar restructured into three visual clusters — template picker, save/rename, credential picker — replacing the previous single long strip of controls.

### Fixed
- Loading a template whose content the form can't represent (e.g. a `sys` section with unsupported keys) no longer wipes the form editor. `hydrateFormFromTemplate()` in `web/src/pages/Provision.svelte` is now atomic: each section is hydrated into a local variable first, and form state is only replaced when every section succeeds. On failure, the view still flips to JSON and a notice is shown — but switching back to Form preserves whatever was already entered.

### Removed
- Dead bulk action `set_24h` (was listed in `validateBulkAction` and `SortedBulkActions` but had no apply/summary path, so any client call silently fell through to "unsupported action").

## [0.0.10] - 2026-04-18

User-facing additions: per-device and per-job export flows, plus an "advanced mode" gate that hides the Provision JSON editor by default. CI also moves to Node-24 action majors ahead of the 2026-06-02 GitHub Actions Node 20 sunset.

### Added
- Settings: "Advanced mode" toggle (off by default). When off, the raw JSON template editor on Provision is hidden so the guided form is the only entry point. Flip it on in Settings → UI Preferences to expose the JSON tab.
- Per-device export endpoint `GET /api/devices/{target}/export` returning a JSON snapshot (`device`, `raw_config`, `raw_status`, `capabilities`). "Export JSON" button added to the device detail page.
- Audit log export endpoint `GET /api/logs/export?format=csv|ndjson` (CSV default, honours the same `level` + `search` filter as `/api/logs`, caps at 100k rows). "Export CSV" and "Export NDJSON" buttons added to the Logs page.

### Changed
- CI: bump GitHub Actions to Node 24–compatible majors (checkout v6, setup-node v6, setup-go v6, docker/* v4–v7) ahead of the 2026-06-02 Node 20 sunset.

## [0.0.9] - 2026-04-17

Review-closure release: closes all 11 findings from the 2026-04-17 project review — no user-facing feature changes, but meaningful reliability, structural, and hygiene improvements across backend and frontend.

### Backend reliability and structure
- Wrapped `UpsertDevices` in a single SQLite transaction so scan/refresh cycles leave the `devices` table consistent if the process is killed mid-loop.
- Replaced ~20 silent `_ = err` patterns in `internal/services/app.go` with explicit `log.Printf` calls so job finalization, JSON marshaling, and scan-payload parsing failures are no longer swallowed.
- Added a graceful-shutdown context to `AppService`: in-flight scan and refresh jobs now observe cancellation and are marked `interrupted` immediately on SIGTERM instead of waiting for the 15s/120s stale-job guard.
- Split `internal/services/app.go` (1317 LoC) into four topic files — `app.go`, `app_jobs.go`, `app_backup.go`, `app_credentials.go` — with zero API or behavior changes.
- Added unit-test coverage for `Provision()` and `ImportBackup()` happy paths and a representative failure per flow.

### Frontend structure and type safety
- Split `web/src/pages/Provision.svelte` (1336 LoC) into per-section sub-components (Sys, MQTT, WS, BLE, Cloud, Matter, Wifi, OTA), each owning its own form state. The JSON editor and credential reference remain peers.
- Tightened `web/src/lib/api.ts` payloads from `unknown`/`object` to named interfaces (`BulkActionRequest`, `ProvisionResult`, `FirmwareUpdateResult`, …) that mirror the Go structs.
- Introduced a Vitest + jsdom harness under `web/` with smoke tests on the API client and provision state helpers (19 tests). CI now runs `npm test` before the build.

### Frontend UX, accessibility, and resilience
- Accessibility pass: added `aria-label` to icon-only buttons (sort indicators, row actions), wrapped decorative glyphs in `aria-hidden="true"`, added `role="alert"`/`role="status"` + `aria-live` regions to error and status panels, and populated `aria-valuenow/min/max` + `aria-busy` on progress bars.
- Consolidated 26 duplicated sortable `<th>` blocks in `Devices.svelte` into a reusable `<SortHeader>` component.
- Added transient-network retry/backoff to the API client — 2 retries with 200/400 ms backoff on idempotent methods only. Mutations and HTTP status errors are never retried.
- Made Vite minification settings explicit (`minify: 'esbuild'`, `target: 'es2020'`, `cssMinify: true`) and added a CI bundle-size budget gate (`web/scripts/check-bundle-size.mjs`) that enforces raw + gzip budgets for the JS and CSS bundles.

### Docs
- Fixed the entry-point path in `CLAUDE.md` (now `cmd/shellyctl/main.go`).
- Documented the new services-file layout in `docs/ARCHITECTURE.md` and the new test/bundle-budget commands in `docs/DEVELOPMENT.md` and `CONTRIBUTING.md`.

## [0.0.8] - 2026-04-16

- Drop Gen1 device support: all HTTP REST (GET-based) Shelly code paths removed from scanner, provisioner, setters, compliance, and frontend. Devices with unknown generation now default to Gen2. Templates containing `gen1_http` sections are gracefully skipped rather than applied.

## [0.0.7] - 2026-04-16

- Fix `RandomSecret()` to panic instead of silently returning a hardcoded fallback when `crypto/rand` is unavailable
- Upgrade `golang.org/x/crypto` to v0.45.0 and `golang.org/x/net` to v0.47.0 (resolves all 5 Dependabot alerts)
- Add CI workflow: `go test ./...` and frontend build run on every push and PR to main
- Add tests for `isProvisionTargetAllowed()` covering all address categories
- Bump frontend package version to match release

## [0.0.6] - 2026-04-16

- Fixed lat/lon values being silently dropped when saving provisioning templates (inputs now use `type=number`)
- Added Delete and Rename template actions directly on the Provision page
- Removed redundant Templates section from Settings (managed on Provision page)
- Aligned section order between Provision and Compliance pages (both now lead with sys)
- Aligned sys field order between pages (lat/lon after RPC UDP Port on both)
- Extended provisioner, scanner, compliance, and setter internals

## [0.0.5] - 2026-04-15

- Public repo readiness work: root security/contributing docs, issue templates, changelog, and local-artifact ignore rules
- Added per-device detail and API docs pages to the embedded UI
- Standardized `Last Success` time presentation across Devices and per-device detail
- Expanded the documented OpenAPI v1 route surface and tightened missing-asset handling

## [0.0.4] - 2026-04-14

- Added configurable refresh timeout handling and stale-device signaling in the Devices view
- Clarified successful refresh timing with `Last Success` wording
- Added database migration support for device refresh-state tracking
- Published a GitHub Actions workflow for GHCR image releases
- Aligned Docker Compose defaults with `ghcr.io/buliwyf42/shellyadmin`

## [0.0.3] - 2026-04-08

- Added delete-all log cleanup in the API and Logs page
- Improved device table UX with auto-refresh, clearer row actions, and more visible IP links
- Added About page version and commit visibility
- Hardened authentication, API mutation handling, and job concurrency behavior
- Expanded docs for deployment, refresh behavior, and architecture alignment
