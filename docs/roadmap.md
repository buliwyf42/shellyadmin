# Roadmap

This is the single source of truth for ShellyAdmin's planned direction. The README
links here for anything beyond the current feature set. Items in this file are intent,
not commitments — scope and timing will shift as the project matures.

For accepted architectural decisions see [adr/README.md](./adr/README.md). For the
threat model and deployment expectations see [SECURITY.md](./SECURITY.md).

## Now (v0.1.x)

- Field-test the v0.1.7–v0.1.12 sweep on a real fleet before adding more
  surfaces. Almost every operator-facing area changed; concrete bug
  reports beat speculative additions.

## Next (pre-v1)

- `shellyctl` CLI once the external API contract has settled enough to build against
  without churn. Will need its own ADR to scope the command surface.
- Handler-side interface extraction and broader unit test coverage on
  `internal/core/scanner`, `internal/core/firmware`, and `internal/core/setters`.
- Review and tighten gin, sessions, and x/crypto dependency pins on a regular cadence.
- Configurable firmware-install poll cadence (currently fixed at 5 s).

## Recently shipped

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
