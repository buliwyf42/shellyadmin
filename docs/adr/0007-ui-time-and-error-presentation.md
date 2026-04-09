# ADR-0007: UI Time and Error Presentation Policy

- Status: `Accepted`
- Date: 2026-04-09

## Context

Operators need diagnostics that stay readable while still exposing technical detail when needed.

## Decision

- Time rendering should be user-switchable between locale display and UTC-style display.
- Error presentation for partial failures should be:
  - concise summary by default
  - expandable technical details on demand

## Consequences

- The UI remains approachable for routine operation but still supports troubleshooting.
- API responses should preserve technical detail fields needed by expandable views.
