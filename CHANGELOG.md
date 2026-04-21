# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

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
