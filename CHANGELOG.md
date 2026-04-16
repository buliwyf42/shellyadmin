# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [0.0.8] - 2026-04-16

- Drop Gen1 device support: all HTTP REST (GET-based) Shelly code paths removed from scanner, provisioner, setters, compliance, and frontend. Devices with unknown generation now default to Gen2. Templates containing `gen1_http` sections are gracefully skipped rather than applied.

## [0.0.7] - 2026-04-16

- Fix `RandomSecret()` to panic instead of silently returning a hardcoded fallback when `crypto/rand` is unavailable
- Upgrade `golang.org/x/crypto` to v0.45.0 and `golang.org/x/net` to v0.47.0 (resolves all 5 Dependabot alerts)
- Add CI workflow: `go test ./...` and frontend build run on every push and PR to main
- Add tests for `isProvisionTargetAllowed()` covering all address categories
- Bump frontend package version to match release

## [0.0.6] - 2026-04-16

- Fixed lat/lon values being silently dropped when saving provisioning templates (inputs now use `type=number`)
- Added Delete and Rename template actions directly on the Provision page
- Removed redundant Templates section from Settings (managed on Provision page)
- Aligned section order between Provision and Compliance pages (both now lead with sys)
- Aligned sys field order between pages (lat/lon after RPC UDP Port on both)
- Extended provisioner, scanner, compliance, and setter internals

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
