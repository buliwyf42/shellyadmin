# ADR-0006: Backup, Export/Import, and Secret Handling

- Status: `Accepted`
- Date: 2026-04-09

## Context

The project needs lightweight, product-level backup/restore behavior aligned with operator expectations.

## Decision

- Provide built-in export/import flows.
- Initial backup scope is:
  - settings
  - templates
- Export format is JSON.
- Restore flow requires dry-run first, then explicit operator decision.
- Secrets policy for export:
  - default export is redacted
  - plaintext secret export is allowed only through an explicit extra confirmation step
- At-rest secrets are currently allowed in DB plaintext.
- Future migration target is encryption at rest using an app-managed master key from environment/file.
- Audit logs for secret-related actions include metadata only and never include secret fields.

## Consequences

- Backup UX is practical for small, single-operator deployments.
- Default secret posture is safer for routine export.
- Encryption-at-rest migration remains compatible with current deployments while providing a defined future direction.
