# Security Policy

ShellyAdmin is intended for trusted local network use by a single trusted operator. It is not designed for direct internet exposure, multi-tenant hosting, or public API use.

The detailed security model lives in [docs/SECURITY.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/SECURITY.md).

## Supported Versions

Security fixes are best-effort while the project is in active early development.

| Version                                    | Supported   | Notes                                                                                                                                 |
| ------------------------------------------ | ----------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `v0.1.18`                                  | Yes         | Current baseline                                                                                                                      |
| `v0.1.17`, `v0.1.16`, `v0.1.15`, `v0.1.13` | Best effort | Recent v0.1.x sweep                                                                                                                   |
| `v0.1.14`                                  | **No**      | Broken release — `go.mod` directive forced 1.25 with CI on 1.24, no GHCR image was published. Upgrade directly to `v0.1.15` or later. |
| `v0.1.7` – `v0.1.12`                       | Best effort | Operational improvements; no known unfixed CVEs                                                                                       |
| `v0.1.4` – `v0.1.6`                        | Best effort | Older v0.1.x — prefer the most recent release                                                                                         |
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
- **Use `SHELLYADMIN_PASS_HASH`** (argon2id PHC from `shellyctl hash-password`) for the admin password. `SHELLYADMIN_PASS` (plaintext) still works but is scheduled for removal in v0.2.0 (no earlier than 2026-07-22).
- Set a strong `SHELLYADMIN_SECRET` for real deployments.
- Prefer `SHELLYADMIN_PASS_HASH_FILE`, `SHELLYADMIN_SECRET_FILE`, and `SHELLYADMIN_ENCRYPTION_KEY_FILE` for containers — keep cleartext out of environment files and container manifests.
- Treat the product as a LAN admin tool, not an internet-facing identity system.

## Hardening notes

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
