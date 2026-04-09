# ADR-0002: Operational Safety and Compliance Model

- Status: `Accepted`
- Date: 2026-04-09

## Context

Operational behavior must match the intended risk posture for a trusted-LAN appliance.

## Decision

- Risky operations remain manual-only.
- Click confirmation is sufficient; no typed phrase is required.
- Compliance is advisory only.
- Compliance UI should show status/issues and provide fix hints, but never auto-apply fixes and never block actions.
- Retry behavior for failed actions is user-triggered (suggested retry), not automatic retry loops.

## Consequences

- Operator intent remains explicit and auditable.
- Compliance remains an alignment aid, not a policy enforcement engine.
- Implementation should avoid hidden automation for firmware/provisioning retries.
