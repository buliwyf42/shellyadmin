# Roadmap

This is the single source of truth for ShellyAdmin's planned direction. The README
links here for anything beyond the current feature set. Items in this file are intent,
not commitments — scope and timing will shift as the project matures.

For accepted architectural decisions see [adr/README.md](./adr/README.md). For the
threat model and deployment expectations see [SECURITY.md](./SECURITY.md).

## Now (v0.1.x)

- Continue field-testing on a real fleet between increments; the v0.1.7–
  v0.1.12 sweep changed almost every operator-facing area, so concrete
  bug reports still beat speculative additions.

## Next (pre-v1)

- `shellyctl` CLI once the external API contract has settled enough to build against
  without churn. Will need its own ADR to scope the command surface.
- Periodic dependency pin review on a regular cadence (next pass: ~3 months
  out, or sooner if a CVE lands).
- **vite.config oxc minifier**: vite 8 made `oxc` the default minifier
  and unbundled `esbuild`. Currently pinning `minify: 'esbuild'` + the
  `esbuild` devDep to keep byte-stable build output across the v0.2.0
  rollup→rolldown jump. Worth a separate task to switch to oxc and
  drop the devDep.

## Recently shipped

### 2026-05-10

- **v0.2.3** — MCP `shellyctl mcp` stdio subcommand for Claude Desktop
  on the same host (ADR-0011 v0.2.3 follow-up). Same 21-tool surface
  exposed on stdin/stdout instead of HTTP+token. Trust boundary is
  the parent process (no transport-level auth); logs to stderr; no
  background workers (query session, not server). Plus `firmware_status`
  paging — adds optional `status`/`has_update`/`search`/`limit`/`offset`
  inputs and `filtered_total`/`returned` outputs so 200+ device fleets
  don't trip per-tool output caps. Per-token scoping was on the
  v0.2.x candidate list but dropped: single-operator deployments
  don't benefit from splitting the surface across multiple tokens.

- **v0.2.2** — Svelte 5 reactivity migration. Closes the four lint
  rules disabled during the v0.2.0 dep bump: `svelte/require-each-key`
  (21 sites across 11 files now carry stable keys),
  `svelte/prefer-svelte-reactivity` (Groups + Provision `selected`
  migrated from `new Set` to `new SvelteSet`),
  `svelte/no-useless-mustaches` (UserCAForm placeholder reformatted),
  `no-useless-assignment` (Provision.svelte 2 false positives
  inline-disabled with comment). `eslint.config.js` disable list is
  empty for the first time since v0.2.0.

- **v0.2.1** — `docker/entrypoint.sh` args passthrough fix. The
  documented `docker run <image> shellyctl hash-password` recipe
  panicked since the entrypoint script was introduced because the
  exec line didn't pass `"$@"`. One-line fix; doc sweep dropped the
  leading `shellyctl` from all docker-run recipes.

- **v0.2.0** — Removes deprecated `SHELLYADMIN_PASS` plaintext env var
  (the v0.0.15 deprecation); `SHELLYADMIN_PASS_HASH` is now the only
  supported entry point. Pulls the deferred-from-v0.1.14 major frontend
  dep updates: TypeScript 5.9 → 6.0, Vite 6 → 8 (rolldown bundler),
  ESLint 9 → 10, plus the matching plugin/parser bumps. Bundle-size
  budget raised from 280 → 300 KB raw / 80 → 86 KB gzip to absorb the
  rollup → rolldown switch (~15 KB raw / ~5 KB gzip). One real
  reactivity bug fixed: `Provision.svelte` `precheckTemplate` reactive
  statement now lists its 17 state deps explicitly so it actually
  re-runs.

- **v0.1.23** — `services.RefreshDevice` now resolves targets by
  Name in addition to MAC/IP. Caught by the v0.1.22 live demo when
  the same `target` worked via `execute_device_action` but failed
  via `refresh_device`. Single-line fix; new test mirrors the
  v0.1.19 `TestGetDeviceDetailResolvesByMACOrIPOrName` pattern.

### 2026-05-09

- **v0.1.22** — State-changing MCP tools, confirm-gated. 8 new tools
  (refresh_device, refresh_all_devices, start_scan, confirm_scan,
  firmware_check, firmware_install, execute_device_action,
  bulk_action) bring fleet management into the MCP surface. Each
  requires explicit `confirm: true` to execute; without it returns
  a typed preview describing what would happen so the LLM can
  summarize and obtain operator approval. Tool descriptions include
  a verbatim "OPERATOR APPROVAL REQUIRED" policy paragraph; audit
  rows tag `mode=preview` vs `mode=confirmed` and carry risk_level.
  Hard-excluded: ShellyAdmin's own config (settings, credentials,
  templates, provisioning, log clearing) — those stay SPA-only.
  ADR-0011 amended with a v0.1.22 follow-up covering the design
  trade-off (single confirm flag vs two-call token dance) and the
  full tool table.

- **v0.1.21** — Live MCP toggle (no restart required).
  v0.1.20's restart-required posture lifted: enabling, disabling, or
  rotating the MCP token in Settings applies in-process without
  restarting the container. New `MCPController` on `services.AppService`
  serializes start/stop/rotate transitions. `models.AppSettings.MCPRunning`
  (read-only) drives a Running/Stopped badge on the MCP card. Came
  with one architectural fix: `api.Config.Service` lets `main.go`
  hand its `*services.AppService` to the router so HTTP handlers and
  background workers share state — the MCP controller surfacing
  forced this; previously each had its own service. 5 new lifecycle
  tests using a `MCPBuilder` test seam (returns httptest listeners
  so parallel test runs don't collide on real ports). ADR-0011
  amended with a v0.1.21 follow-up section.

- **v0.1.20** — Settings UI for MCP + page reorganization.
  `models.AppSettings` gains `MCPEnabled` and `MCPToken` (encrypted at
  rest via `internal/core/secretbox`); the SPA's Settings page can now
  enable, disable, and rotate the MCP token without touching the
  container's `docker run`. `SHELLYADMIN_MCP_TOKEN` env var still
  takes precedence (operator override; UI shows a "managed by
  environment variable" notice with controls disabled). `cmd/shellyctl/main.go`
  resolution order: env → settings → disabled. API GET redacts the
  persisted token to `<set>`; sending `<set>` back on save preserves
  the existing stored value (round-trip without exposure). Settings
  page reorganized from 3 mixed cards into 5 focused cards (Discovery
  & Refresh, Firmware, MCP, Display, Backup). MCP card has Show /
  Hide / Generate (CSPRNG, 64 hex chars) / Copy / Clear actions on
  the token input, with per-state hint text. ADR-0011 amended with
  a follow-up section documenting the precedence rule, encryption
  approach, and the restart-required-vs-live-toggle decision.

- **v0.1.19** — Optional read-only MCP server. New `internal/mcp` package
  embedded in the existing binary; binds on `:8081` only when
  `SHELLYADMIN_MCP_TOKEN` is set, off otherwise. 13 tools (list_devices,
  get_device, list_device_actions, scan_status, firmware_status,
  firmware_install_status, list_templates, get_template, list_credentials
  (redacted), get_settings, get_logs, export_device, compliance_summary)
  as thin adapters over `services.AppService`. Static token auth via
  either `Authorization: Bearer <token>` header **or** first URL path
  segment (`http://host:8081/<token>/`, the same shape Home Assistant's
  MCP integration uses for `mcp-remote`-style clients) — both run
  through `subtle.ConstantTimeCompare`; `X-Request-ID` honored and
  audit lines flow through `service.LogCtx` so MCP activity shows up
  in `/api/logs` with `mcp ` prefix. Hard exclusion: anything that
  mutates state. Picks the official
  `github.com/modelcontextprotocol/go-sdk` v1.6.0 (just hit v1.0;
  typed-generic `mcp.AddTool` auto-generates JSON schemas from input
  structs). Dep-bump-trap check passes — top entries stay at `1.25.0`.
  Same-day post-deploy refinements: `scan_status.pending` slimmed to a
  6-field summary (~63 KB → ~7.5 KB on a 44-device fleet) so the response
  fits in MCP client output caps; `services.GetDeviceDetail` now resolves
  by name in addition to MAC/IP, fixing `get_device` /
  `list_device_actions` / `export_device` for name-based lookups.
  Design rationale in [adr/0011-mcp-read-only-server.md](./adr/0011-mcp-read-only-server.md).

### 2026-05-08

- **v0.1.18** — Setters round-out + provisioning integration smoke
  (final M3 step). 6 setters-test groups closing the gaps in lat/lon
  payload shape, percent clamping, method-not-found behavior, and the
  `BLEPair` (ok, supported, message) tri-state. New
  `TestProvisionDevice_MultiSectionSmoke` drives sys + mqtt + wifi +
  auth in one `ProvisionDevice` call and pins the
  `Shelly.SetAuth` HA1 calculation. `internal/core/setters` coverage
  32.1% → 56.4%; `internal/core/provisioner` → 61.7%.
- **v0.1.17** — Firmware + scanner unit tests. New shared `fakeShelly`
  test fixture; 10 firmware tests + 3 methods tests + 9 auto-update
  tests + 5 scanner clock/failure tests. firmware package coverage
  jumps from 0% to 71.1%; scanner from 21% to 39.2% (the JSON-RPC
  paths are covered; CIDR/mDNS/concurrency intentionally out of
  scope). Adds `ReadAutoUpdateOnClient` to mirror the existing
  `SetAutoUpdateOnClient` precedent.
- **v0.1.16** — Platform refresh: Go 1.25. Bumped CI workflow + Dockerfile
  base + go.mod directive from 1.24 to 1.25; re-took the v0.1.14 dep
  upgrades that needed it (gin v1.12, gin-contrib/sessions v1.1.0,
  x/net v0.51, x/text v0.35, x/sync v0.20). HTTP/3 transitive deps
  (quic-go) come along for the ride; not currently used by ShellyAdmin.
- **v0.1.15** — Testability seams + v0.1.14 CI rollback. New
  `internal/core/clock` package (`Clock` interface + `Real()` +
  `Fake.Advance`); `OnClient` variants on scanner / firmware / setters
  that accept a pre-built `shellyclient.Client`; `Clock` field on
  `scanner.ProbeOptions` and `firmware.Options`. Three bare
  `time.Now()` sites replaced. Also rolls back v0.1.14's gin/x-net/
  x-text/x-sync bumps to restore Go 1.24 compatibility (v0.1.14's
  dep bumps had pulled `quic-go` and forced `go 1.25.0`, breaking CI;
  no GHCR image was published for v0.1.14).
- **v0.1.14** — Security hygiene. **GHCR image never published** —
  the dep bumps inadvertently forced `go 1.25.0` and CI Test +
  Publish-Image both failed. Upgrade path: v0.1.13 → v0.1.15. The
  plaintext-deprecation-warning sharpening from this release is
  preserved in v0.1.15. Plaintext-password deprecation warning
  sharpened with a concrete removal target (v0.2.0, no earlier than
  2026-07-22; mirrored in `docs/SECURITY.md`). Conservative dep bumps
  (patch + minor only): `gin` 1.10.1 → 1.12.0, `gin-contrib/sessions`
  1.0.2 → 1.1.0, `gorilla/sessions` (indirect) 1.2.2 → 1.4.0,
  `golang.org/x/crypto` 0.45.0 → 0.48.0; npm in-range patches across
  the TS/eslint/vitest toolchain. Majors deferred.
- **v0.1.13** — Configurable firmware-install poll cadence. The
  install_job's per-device version-recheck loop is now an AppSetting
  (`firmware_install_poll_interval`, default 5 s, bounded `[1, 60]`).
  Surfaced on the Settings page next to Install timeout. Helper +
  Normalize unit-tested.

### 2026-05-07 (intra-day burst v0.1.8 → v0.1.12)

- **v0.1.12** — Logs page risk filter; `batch` + `fw_id` (long firmware
  identifier with build hash) on the device detail page; Devices Model
  column sort keys on the displayed text.
- **v0.1.11** — Friendly device labels via Shelly's `app` field
  (`PlugSG3` etc.) shown as primary on Devices/Firmware pages, model
  SKU + Gen + component counts in hover tooltip; small badge on the
  device detail page header; Type / Model SKU / Components rows in
  the Status grid.
- **v0.1.10** — Capabilities column on Devices (switch/cover/light
  counts derived from `RawStatus`); structured `risk_level` on every
  audit row written for an action execution; CSV export gains the
  column. Threaded via context so non-action audit sites stay
  unchanged.
- **v0.1.9** — Per-component action fan-out (Cover open/close/stop,
  Switch toggle, Light toggle per `<type>:N` instance), `ota_revert`
  with typed-name confirm. Closes the v2 wave of ADR-0010.
- **v0.1.8** — Per-device action discovery via `Shelly.ListMethods`
  (catalog refactor, four new fleet-wide actions: `wifi_scan`,
  `eth_status`, `factory_reset_wifi`, `factory_reset`); typed-name
  confirm modal; ADR-0010 promoted from plan.

### 2026-05-06

- **v0.1.7** — Drop legacy `fw_status` / `fw_available_ver` columns
  (migration 019); drop legacy plaintext credential columns (020);
  scheduler + install-timeout helpers extracted and tested;
  auto-release pipeline gains em-dash subtitle support.
- **v0.1.6** — auto-update via `Schedule.*`
  (read/write/bulk/compliance/provisioner); Refresh now also syncs
  firmware data; CI on golangci-lint v2 + Node.js 24.
- **v0.1.5** — Firmware page rebuild (dual-channel cache,
  `firmware_install` job, confirmation modal, sortable table);
  out-of-band drift detection via `Shelly.GetDeviceInfo`;
  configurable Gen badge colors; shared Stable/Beta channel between
  Devices and Firmware pages.

### Done in earlier release windows

- CI tightening (golangci-lint, eslint, prettier, bundle-size budget)
- Scan target restriction to RFC1918 / link-local
- `Store` service/DB boundary interface
- Per-device bulk-action audit fidelity

## v1.0.0 Gate

- API stability guarantee: semver applies from v1.0.0 onward. v0.x remains subject
  to breaking changes.
- Documented upgrade path from the latest v0.x to v1.0.0.

## Explicitly not planned

- Multi-user RBAC.
- Direct internet exposure or hardened WAN deployment.
- High-availability or clustered deployment.
- Automated self-healing flows beyond the current manual, previewed actions.
