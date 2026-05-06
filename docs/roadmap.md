# Roadmap

This is the single source of truth for ShellyAdmin's planned direction. The README
links here for anything beyond the current feature set. Items in this file are intent,
not commitments — scope and timing will shift as the project matures.

For accepted architectural decisions see [adr/README.md](./adr/README.md). For the
threat model and deployment expectations see [SECURITY.md](./SECURITY.md).

## Now (v0.1.x)

- Broader action discovery for device components where protocol support is reliable.
  Requires surveying per-component RPC availability before exposing it in the UI so
  we do not ship actions that silently fail on specific models.
- Drop the legacy plaintext `password` / `ha1` columns on `credentials` and
  `credential_groups` once enough releases have shipped with the cipher migration
  in place to make rollback unnecessary. Landing this requires a migration that
  refuses to downgrade.
- Drop the legacy `fw_status` / `fw_available_ver` columns left orphaned by
  v0.1.5's per-channel rebuild (kept around for one rollback window).

## Next (pre-v1)

- `shellyctl` CLI once the external API contract has settled enough to build against
  without churn.
- Handler-side interface extraction and broader unit test coverage on
  `internal/core/scanner`, `internal/core/firmware`, and `internal/core/setters`.
- Review and tighten gin, sessions, and x/crypto dependency pins on a regular cadence.
- Configurable firmware-install timeout (currently fixed at 5 minutes per device).
- Scheduled firmware checks (currently manual-only via "Check Firmware" or piggybacked on Refresh).

## Recently shipped

- v0.1.6 — auto-update via `Schedule.*` (read/write/bulk/compliance/provisioner); Refresh now also syncs firmware data; CI on golangci-lint v2 + Node.js 24.
- v0.1.5 — Firmware page rebuild (dual-channel cache, `firmware_install` job, modal, sortable table); out-of-band drift detection via `Shelly.GetDeviceInfo`; configurable Gen badge colors; shared Stable/Beta channel.
- Done long ago: CI tightening (golangci-lint, eslint, prettier, bundle-size), scan-target restriction, `Store` service/DB boundary, per-device bulk-action audit fidelity.

## v1.0.0 Gate

- API stability guarantee: semver applies from v1.0.0 onward. v0.x remains subject
  to breaking changes.
- Documented upgrade path from the latest v0.x to v1.0.0.

## Explicitly not planned

- Multi-user RBAC.
- Direct internet exposure or hardened WAN deployment.
- High-availability or clustered deployment.
- Automated self-healing flows beyond the current manual, previewed actions.
