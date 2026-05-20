# ADR-0016: `shellyctl` Operator CLI (HTTP + PAT, Read-Only First)

- Status: Accepted
- Date: 2026-05-20

## Context

The binary already ships three flat subcommands (`hash-password`, `mcp`,
`unlock`). Operators have asked for a scriptable way to inspect the fleet
from a shell — list devices, read a device's detail, tail the audit log —
without driving the SPA or hand-crafting `curl` calls against `/api`.

Two transports were possible:

1. **Local SQLite access** (the `mcp` stdio model): open the database file
   directly. No server required, but host-file-access only, no remote use,
   and it duplicates the read paths the API already exposes.
2. **HTTP API + Personal Access Token**: call the running instance over
   `/api`, authenticated by a `pat_…` bearer token. Works remotely, reuses
   the existing scope-gated read endpoints and PAT auth (Block 4c / T3), and
   carries zero new data-access surface.

## Decision

Build `shellyctl` as an **HTTP-API client authenticated with a Personal
Access Token**, **read-only in its first version**.

- **Transport:** every command issues an HTTP GET against
  `--url` (default `http://localhost:8080`, env `SHELLYADMIN_URL`) with
  `Authorization: Bearer <token>` (`--token`, env `SHELLYADMIN_TOKEN`). No
  direct DB access; the server is the single source of truth and the PAT
  scope catalog is the single authorization model.
- **First command surface (read-only):** `devices` (list), `device
  <mac|ip|name>` (detail), `logs` (audit tail). All map 1:1 to existing
  `GET /api/devices`, `GET /api/devices/:target`, `GET /api/logs`. They need
  only the `devices:read` / `admin` scopes already defined for PATs.
- **Output:** human-aligned tables by default (stdlib `text/tabwriter`),
  `--json` to emit the raw API payload for piping into `jq`.
- **Dispatch:** `cmd/shellyctl/main.go` routes a known CLI verb to
  `internal/cli.Run`; bare `shellyctl` still starts the server.

State-changing commands (refresh, scan, firmware install, bulk actions) are
**explicitly out of scope** for this version. They can be added later behind
their respective `*:write` scopes once the read-only grammar has settled —
mirroring how the MCP server shipped read-only first (ADR-0011) before its
v0.1.22 confirm-gated write tools.

## Consequences

- No new authorization or data-access code: the CLI is a thin client over
  endpoints and a token model that already exist and are already tested.
- Remote-capable out of the box; the same binary that runs the server can
  query a different instance by pointing `--url` at it.
- A PAT must exist (minted in the Settings UI). The CLI cannot mint, list,
  or revoke PATs — the privilege-escalation guard on the PAT endpoints
  (Block 4c) applies to bearer-authed callers regardless.
- The CLI decodes only the fields it renders (local response structs), so a
  new field on `models.Device` does not break it and it stays decoupled from
  the server's struct evolution.
- Future write commands inherit the same transport; the read-only line in
  the sand keeps this first increment low-risk.

## Related Work

- ADR-0011 (read-only MCP server) — same "read-only first, writes later
  behind explicit gating" staging.
- Block 4c / T3 (Personal Access Tokens) — the scope catalog and bearer auth
  this CLI rides on.
