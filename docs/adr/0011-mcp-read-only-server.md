# ADR-0011: Read-Only MCP Server (Embedded HTTP, Bearer-Token Gated)

- Status: `Accepted`
- Date: 2026-05-09
- Implements: v0.1.19
- Roadmap link: [docs/roadmap.md](../roadmap.md) → "Recently shipped 2026-05-09"
- Related: ADR-0001 (Product Scope and Non-Goals), ADR-0003 (Device Auth and Credentials)

## Context

ShellyAdmin currently exposes one programmatic surface — the cookie + CSRF
gated Gin HTTP API at `:8080`, primarily there to back the embedded SPA.
With LLM-driven agents (Claude Desktop, Claude Code, custom MCP clients)
becoming a routine part of operator workflows, there is value in letting an
agent introspect the fleet — *what devices are on the network, which are
out of compliance, is a firmware check still running* — without scraping
the SPA or hand-rolling cookie/CSRF flows in a tool harness.

Two constraints frame the design:

1. **Operator safety.** ADR-0001 pins ShellyAdmin as a *trusted-LAN, single
   operator* tool. Action surfaces (provision, firmware update, credential
   mutation, settings writes) are explicitly required to remain "explicit
   and reviewable." A programmatic surface that can shut a relay off or
   push firmware without an operator-driven confirmation loop is a step
   away from that posture.
2. **Roadmap alignment.** The roadmap's "Next (pre-v1)" already lists a
   `shellyctl` CLI as a sibling future-exposure of the same `services.AppService`
   surface, with the explicit note *"Will need its own ADR to scope the
   command surface."* MCP is the same shape of decision and earns the same
   ADR treatment.

## Decision

Add an opt-in, **read-only** MCP server embedded in the existing `shellyctl`
binary, gated on a static bearer token.

### Scope (v1)

13 read-only tools, each a thin adapter over a public `services.AppService`
method:

| Tool | Backed by |
|------|-----------|
| `list_devices` (with `search` / `gen` / `limit` filters) | `GetDevices()` |
| `get_device` | `GetDeviceDetail(target)` |
| `list_device_actions` | `ListDeviceActions(target)` |
| `export_device` | `ExportDevice(target)` |
| `scan_status` | `ScanStatus()` |
| `firmware_status` | `FirmwareStatus()` |
| `firmware_install_status` | `FirmwareInstallStatus()` |
| `list_templates` | `ListTemplates()` |
| `get_template` | `GetTemplate(name)` |
| `list_credentials` | `ListCredentials()` → redactor |
| `get_settings` | `GetSettings()` |
| `get_logs` (with `level` / `search` / `risk` / `limit`) | `GetLogsFiltered(...)` |
| `compliance_summary` | `GetDevices()` × `compliance.Evaluate` |

### Hard exclusions in v1

Refresh, scan trigger, scan confirm, firmware check, firmware update, firmware
install, provision, upload-CA, save/delete templates, save/delete credentials,
save settings, clear logs, run bulk action, set auto-update. State-changing
tools are deferred to a future ADR; they need a confirmation/audit-trail
design that v1 does not provide.

### Transport, port, auth

- **Streamable HTTP** via `github.com/modelcontextprotocol/go-sdk` v1.6.0 —
  the official Go MCP SDK; just hit v1.0; typed-generic `mcp.AddTool[In, Out]`
  auto-generates JSON schemas from input structs.
- **Separate listener on `:8081`** (configurable: `SHELLYADMIN_MCP_PORT`,
  `SHELLYADMIN_MCP_BIND`). Keeps the MCP auth path off the cookie + CSRF
  middleware chain that protects the SPA, and isolates the new failure mode
  (MCP listener won't bind / panics) from the main UI.
- **Static bearer token** via `SHELLYADMIN_MCP_TOKEN` (resolved through
  `services.DecodeSecretValue` → supports `_FILE` indirection). When unset,
  the listener does not start and an info log is emitted at boot. There is
  no other way to enable MCP — this is the gate.
- **Two equivalent transport-level auth shapes**, picked per client:
  - `Authorization: Bearer <token>` header (default; spec-conformant).
  - URL whose first path segment IS the token, e.g.
    `http://host:8081/<token>/`. The matched prefix is stripped before the
    request reaches the SDK handler. Convenient for MCP clients (notably
    `mcp-remote`) where a header arg is awkward and the operator prefers
    one URL string in the config — the same shape Home Assistant's MCP
    integration uses. Both comparisons run through `subtle.ConstantTimeCompare`;
    headers are checked first and take precedence.

### Secret hygiene

`list_credentials` and `get_settings` route their `AppService` results through
a small redactor in `internal/mcp/redact.go` so plaintext passwords and HA1
hashes never reach an MCP client. A dedicated test
(`internal/mcp/redact_test.go`) verifies a marshalled credential output never
contains the seeded plaintext.

### Audit logging

Every tool call writes through `service.LogCtx(ctx, ...)`. A small middleware
populates `X-Request-ID` from the inbound header (sanitized to `[A-Za-z0-9_-]{1,64}`)
or generates a fresh 16-hex-char id, then puts it in the request context via
`middleware.WithRequestID`. MCP entries appear in `/api/logs` filterable by
request id, prefixed with `mcp `.

## Alternatives Considered

- **Stdio sub-command (`shellyctl mcp`).** Cleaner for Claude Desktop on
  the same host, but awkward for the project's actual deployment shape
  (Docker container on `docker.home.lan`, accessed remotely from a Mac).
  Listed for v0.2.x.
- **Full surface, including state-changing tools.** Maximum capability but
  conflicts with ADR-0001's "explicit and reviewable" requirement. Deferred
  pending a confirmation-flow design.
- **Reuse the existing session cookie.** Single source of truth for auth,
  but typical MCP clients are token-oriented. The mismatch is not worth
  the consistency win for a v1 that's already behind a config flag.
- **Loopback-only, no auth.** Would block access from a Mac to the home-lab
  Docker host — too restrictive for the actual deploy shape.

## Consequences

- **API surface enters the v1.0 stability commitment.** The roadmap's "v1.0.0
  Gate" promises semver from v1.0.0 onward. The MCP tool names, input shapes,
  and output shapes shipped here are part of what v1.0.0 will guarantee.
- **No state-changing operations from agents (yet).** v0.2.x will revisit
  this; the redactor + audit + bearer-gate scaffolding lands here so the
  next pass only adds tools, not infrastructure.
- **Minimal ops cost when off.** Default-off; when the env var is unset the
  listener does not bind and no goroutine is spawned. Operators who don't
  use MCP carry zero runtime cost beyond the small dep-graph addition.

## Implementation

- New package `internal/mcp/` (server, auth, tools, redact, tests).
- Wired into `cmd/shellyctl/main.go` as a second goroutine alongside the
  existing API listener.
- New env vars: `SHELLYADMIN_MCP_TOKEN`, `SHELLYADMIN_MCP_PORT` (default
  `8081`), `SHELLYADMIN_MCP_BIND` (default `0.0.0.0`).
- Dockerfile: `EXPOSE 8081` added; existing `:8080/health` healthcheck
  unchanged.
- `docker-compose.yml`: commented-out `8081:8081` mapping and
  `SHELLYADMIN_MCP_TOKEN` env line so operators see the opt-in path.

## Same-day refinements (2026-05-09, post-deploy smoke)

Two issues surfaced by the first-day live smoke against a 44-device fleet,
fixed before the v0.1.19 tag:

- **`scan_status` payload size.** The underlying `services.ScanStatus.Pending`
  is `[]map[string]any` and carries the full `models.Device` shape including
  `supported_methods` (~150-entry RPC list per device). At 43 pending entries
  the response was ~63 KB and tripped MCP client per-tool output caps. The
  MCP adapter now collapses each pending entry to a typed `ScanPendingItem`
  with `{mac, ip, name, model, gen, app}` only — ~88% smaller. The SPA shape
  is unchanged.
- **`get_device` / `list_device_actions` / `export_device` did not resolve
  by name.** `services.GetDeviceDetail` was checking only `MAC` and `IP`,
  contradicting the tool descriptions. Added `Name` to the comparison
  (`internal/services/device_surface.go`); fix propagates to all four
  callers since they share the same lookup.
- **URL-path auth form** added per the alternatives section — see the
  Transport, port, auth section above. Same security model, more ergonomic
  for `mcp-remote`-style clients.
