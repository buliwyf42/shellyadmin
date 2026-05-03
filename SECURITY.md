# Security Policy

ShellyAdmin is intended for trusted local network use by a single trusted operator. It is not designed for direct internet exposure, multi-tenant hosting, or public API use.

The detailed security model lives in [docs/SECURITY.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/SECURITY.md).

## Supported Versions

Security fixes are best-effort while the project is in active early development.

| Version | Supported |
| --- | --- |
| `v0.1.2` | Yes |
| `v0.1.1` | Best effort (superseded — partial false-positive fix only) |
| `v0.1.0` | No (scanner false-positive bug — upgrade) |
| `v0.0.16` | Best effort |
| Older versions | No |

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
- Set strong `SHELLYADMIN_PASS` and `SHELLYADMIN_SECRET` values for real deployments.
- Prefer `SHELLYADMIN_PASS_FILE` and `SHELLYADMIN_SECRET_FILE` for containers.
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
