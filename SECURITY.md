# Security Policy

ShellyAdmin is intended for trusted local network use by a single trusted operator. It is not designed for direct internet exposure, multi-tenant hosting, or public API use.

The detailed security model lives in [docs/SECURITY.md](docs/SECURITY.md).

## Supported Versions

Security fixes are best-effort while the project is in active early development.

| Version                                    | Supported   | Notes                                                                                                                                 |
| ------------------------------------------ | ----------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `v0.3.4`                                   | Yes         | Current baseline. Clear-Logs trigger fix + Dependabot grouping/auto-merge; base images bumped to `node:26-alpine` / `golang:1.26-alpine`. |
| `v0.3.1` – `v0.3.3`                         | Best effort | `runtime_locks` hardening (same-hostname fast path, 60 s window) + TOTP QR enrollment + CI hygiene.                                   |
| `v0.3.0`                                   | Best effort | **Breaking**: external encryption key now required (ADR-0013); single-instance lock enforced (ADR-0015). Adds TOTP 2FA + PATs.        |
| `v0.2.9`                                   | Best effort | Deploy-workflow doc refresh + WebhooksForm a11y warning fix (UI-only; no behavior change).                                            |
| `v0.2.8`                                   | Best effort | Dep pin refresh — closes GO-2026-4918 (non-reachable) in `golang.org/x/net`; `alpine:3.19` → `alpine:3.21` runtime.                   |
| `v0.2.7`                                   | Best effort | Vite oxc minifier swap + drop `esbuild` devDep (build-tooling only; no runtime change).                                               |
| `v0.2.6`                                   | Best effort | Zigbee operations form (UI-only; write-mostly).                                                                                       |
| `v0.2.5`                                   | Best effort | Cover (slat-tilt) provisioner form (UI-only).                                                                                         |
| `v0.2.4`                                   | Best effort | Webhooks provisioner form (UI-only).                                                                                                  |
| `v0.2.3`                                   | Best effort | MCP `shellyctl mcp` stdio subcommand + `firmware_status` paging.                                                                      |
| `v0.2.2`                                   | Best effort | Closes the four lint rules deferred during v0.2.0 dep bump (Svelte 5 reactivity migration).                                           |
| `v0.2.1`                                   | Best effort | Entrypoint args passthrough fix — `docker run <image> hash-password` now works.                                                       |
| `v0.2.0`                                   | Best effort | **Breaking**: `SHELLYADMIN_PASS` plaintext removed — use `SHELLYADMIN_PASS_HASH`. Entrypoint bug — see v0.2.1.                        |
| `v0.1.19` – `v0.1.23`                      | Best effort | Recent v0.1.x sweep (MCP server work)                                                                                                 |
| `v0.1.15` – `v0.1.18`                      | Best effort | Older v0.1.x — prefer the most recent release                                                                                         |
| `v0.1.14`                                  | **No**      | Broken release — `go.mod` directive forced 1.25 with CI on 1.24, no GHCR image was published. Upgrade directly to `v0.1.15` or later. |
| `v0.1.7` – `v0.1.13`                       | Best effort | Operational improvements; no known unfixed CVEs                                                                                       |
| `v0.1.4` – `v0.1.6`                        | Best effort | Older v0.1.x                                                                                                                          |
| `v0.1.0` – `v0.1.3`                        | No          | Scanner false-positive / firmware-page bugs — upgrade                                                                                 |
| `v0.0.16` and older                        | No          |                                                                                                                                       |

## Reporting a Vulnerability

Please do not open a public issue for a suspected security problem.

Use GitHub's private vulnerability reporting flow for this repository if it is enabled. If that is unavailable, contact the maintainer privately before sharing details publicly.

When reporting, include:

- the affected version or container tag
- whether the instance was running on a trusted LAN, behind a reverse proxy, or exposed more broadly
- clear reproduction steps
- expected impact

## Deployment Expectations

- Keep ShellyAdmin on a trusted LAN or behind a private reverse proxy.
- **Use `SHELLYADMIN_PASS_HASH`** (argon2id PHC from `shellyctl hash-password`) for the admin password. The deprecated plaintext `SHELLYADMIN_PASS` was removed in v0.2.0.
- Set a strong `SHELLYADMIN_SECRET` for real deployments.
- Prefer `SHELLYADMIN_PASS_HASH_FILE`, `SHELLYADMIN_SECRET_FILE`, and `SHELLYADMIN_ENCRYPTION_KEY_FILE` for containers — keep cleartext out of environment files and container manifests.
- Treat the product as a LAN admin tool, not an internet-facing identity system.

## Hardening notes

- **v0.2.8** — Periodic dep pin review pulled forward from 2026-08-11.
  Bumps `golang.org/x/net` v0.51.0 → v0.54.0 (closes GO-2026-4918,
  HTTP/2 transport infinite loop on bad `SETTINGS_MAX_FRAME_SIZE`;
  govulncheck confirms no reachable call site in ShellyAdmin code,
  so this is defense in depth rather than a remediated active CVE) and
  `golang.org/x/crypto` v0.48.0 → v0.51.0. Runtime container base moves
  `alpine:3.19` → `alpine:3.21` — 3.19 reached end-of-community-support
  in November 2025 so its apk packages no longer receive security
  updates; 3.21 is supported through November 2026.
- **v0.2.0** — `SHELLYADMIN_PASS` (plaintext admin password) was removed.
  v0.0.15 added `SHELLYADMIN_PASS_HASH` (argon2id PHC from
  `shellyctl hash-password`) and started warning on plaintext use; v0.2.0
  closes the deprecation window. Missing `SHELLYADMIN_PASS_HASH` now panics
  at startup with a pointer to the helper. The operator-facing migration is:
  run `shellyctl hash-password <plaintext>` once and replace the env var.
- **v0.1.0** — Adapts to Shelly firmware **2.0.0-beta1**. Adds an RFC 7616
  Digest auth client (replacing bare unauthenticated probes), per-device HTTPS
  scheme detection with strict TLS-cert-date validation by default (per-device
  opt-out flag for self-signed certs), and 429 brute-force-lockout signalling
  so a wrong-credential refresh stops retrying instead of locking the device
  out for the operator. Strips the removed `ble.enable` flag at provision time
  to avoid the device's stricter validator. New compliance fields
  `enhanced_security`, `tls_cert_valid`, `wifi_hostname` are evaluated only on
  devices that report the underlying state, so 1.x fleets stay green.
- **v0.0.16** — The undocumented `${ENV:...}` env-var expansion in provisioning
  templates has been removed. It previously let an authenticated admin exfiltrate
  server environment variables (including `SHELLYADMIN_PASS_HASH`,
  `SHELLYADMIN_SECRET`, and `SHELLYADMIN_ENCRYPTION_KEY`) by POSTing a crafted
  template to an attacker-controlled LAN IP. Only the documented `{device_name}`
  token remains.
