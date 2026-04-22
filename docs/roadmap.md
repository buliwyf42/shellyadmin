# Roadmap

This is the single source of truth for ShellyAdmin's planned direction. The README
links here for anything beyond the current feature set. Items in this file are intent,
not commitments — scope and timing will shift as the project matures.

For accepted architectural decisions see [adr/README.md](./adr/README.md). For the
threat model and deployment expectations see [SECURITY.md](./SECURITY.md).

## Now (v0.0.x)

- Broader action discovery for device components where protocol support is reliable.
  Requires surveying per-component RPC availability before exposing it in the UI so
  we do not ship actions that silently fail on specific models.
- Tighten CI with Go (`golangci-lint`, `go vet`) and frontend (ESLint, Prettier) checks.
- Restrict scan targets to RFC1918 / link-local networks and cap CIDR size to close
  the DoS surface from very large subnets.
- Introduce a `Store` interface at the service/DB boundary to enable unit tests
  against a fake database.
- Raise bulk-action audit fidelity so per-device outcomes (including the MAC addresses
  affected) are recoverable from the audit log alone.

## Next (pre-v1)

- `shellyctl` CLI once the external API contract has settled enough to build against
  without churn.
- Handler-side interface extraction and broader unit test coverage on
  `internal/core/scanner`, `internal/core/firmware`, and `internal/core/setters`.
- Review and tighten gin, sessions, and x/crypto dependency pins on a regular cadence.
- Drop the legacy plaintext `password` / `ha1` columns on `credentials` and
  `credential_groups` once a release has shipped with the one-shot cipher migration
  in place. Landing this requires a migration that refuses to downgrade.

## v1.0.0 Gate

- API stability guarantee: semver applies from v1.0.0 onward. v0.x remains subject
  to breaking changes.
- Documented upgrade path from the latest v0.x to v1.0.0.

## Explicitly not planned

- Multi-user RBAC.
- Direct internet exposure or hardened WAN deployment.
- High-availability or clustered deployment.
- Automated self-healing flows beyond the current manual, previewed actions.
