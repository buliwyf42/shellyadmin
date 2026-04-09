# ADR-0005: Data, Migration, and Compatibility Policy

- Status: `Accepted`
- Date: 2026-04-09

## Context

The project needs clear expectations for API stability, schema evolution, and startup behavior.

## Decision

- API compatibility policy is best-effort stable.
- Breaking API changes are allowed only with migration/release notes.
- Database migrations are forward-only.
- On migration failure at startup, attempt backup before deciding startup outcome.
- Inventory lifecycle stays manual (devices are retained until explicitly removed).

## Consequences

- Engineering can evolve quickly while still communicating breakage clearly.
- Rollback complexity is intentionally avoided at the schema layer.
- Startup/migration code must implement a backup attempt path before failing fast.
