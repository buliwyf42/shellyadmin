# ADR-0001: Product Scope and Explicit Non-Goals

- Status: `Accepted`
- Date: 2026-04-09

## Context

ShellyAdmin needs a stable product boundary so implementation decisions stay consistent.

## Decision

ShellyAdmin is:

- a single-user operations console
- for trusted LAN environments only
- optimized for small fleets (fewer than 100 devices)

Explicit non-goals:

- multi-user/RBAC support
- internet-facing/public API exposure hardening as a primary target
- multi-tenant operation
- external API token model
- HA/cluster control plane

## Consequences

- Security and UX are optimized for one trusted operator, not tenant isolation.
- Deployment and support guidance can prioritize local/private networking.
- Future features that imply multi-user internet exposure or clustering require a new ADR.
