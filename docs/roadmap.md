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
- **M3 follow-ups (v0.1.16, v0.1.17):** add scanner + firmware unit tests
  using the new Clock + OnClient seams (target: firmware coverage 0% →
  ≥60% on the JSON-RPC translation paths); fill setters_test.go gaps;
  add an integration smoke covering a typical provisioning flow.
- Periodic dependency pin review on a regular cadence (next pass: ~3 months
  out, or sooner if a CVE lands).
- **v0.2.0 cut:** removes `SHELLYADMIN_PASS` plaintext support; pulls major
  dep updates (eslint 10, vite 8, typescript 6, etc.) that were deferred
  in v0.1.14. Earliest target: 2026-07-22.

## Recently shipped

### 2026-05-08

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
