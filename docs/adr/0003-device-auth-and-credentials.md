# ADR-0003: Device Authentication and Credentials Roadmap

- Status: `Accepted`
- Date: 2026-04-09

## Context

Current probing is largely unauthenticated, but future device-level authentication is expected.

## Decision

- Add future support for authenticated device operations.
- Persist `auth_required` state per device in inventory.
- Persist an auth error reason per device (for example `401`, `timeout`).
- Credential ownership model is per template/group.
- Template binding uses exactly one primary credential reference.
- Credential object minimum fields:
  - `name`
  - `username`
  - `password` and/or `ha1`
  - optional tags
- Precheck behavior for mixed targets:
  - skip obviously incompatible devices using known criteria (generation/model/auth state)
  - do not require active preflight probing for every target
- Provisioning auth failure semantics:
  - fail the affected section
  - continue with remaining sections for that device

## Consequences

- The system can surface auth mismatch state without turning unreachable/auth failures into generic offline noise.
- Credential references can be reused cleanly across templates/groups.
- Section-level tolerance preserves partial progress while still exposing failure detail.
