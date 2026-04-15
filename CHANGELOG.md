# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [0.0.5] - 2026-04-15

- Public repo readiness work: root security/contributing docs, issue templates, changelog, and local-artifact ignore rules
- Added per-device detail and API docs pages to the embedded UI
- Standardized `Last Success` time presentation across Devices and per-device detail
- Expanded the documented OpenAPI v1 route surface and tightened missing-asset handling

## [0.0.4] - 2026-04-14

- Added configurable refresh timeout handling and stale-device signaling in the Devices view
- Clarified successful refresh timing with `Last Success` wording
- Added database migration support for device refresh-state tracking
- Published a GitHub Actions workflow for GHCR image releases
- Aligned Docker Compose defaults with `ghcr.io/buliwyf42/shellyadmin`

## [0.0.3] - 2026-04-08

- Added delete-all log cleanup in the API and Logs page
- Improved device table UX with auto-refresh, clearer row actions, and more visible IP links
- Added About page version and commit visibility
- Hardened authentication, API mutation handling, and job concurrency behavior
- Expanded docs for deployment, refresh behavior, and architecture alignment
