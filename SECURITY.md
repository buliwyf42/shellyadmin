# Security Policy

ShellyAdmin is intended for trusted local network use by a single trusted operator. It is not designed for direct internet exposure, multi-tenant hosting, or public API use.

The detailed security model lives in [docs/SECURITY.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/SECURITY.md).

## Supported Versions

Security fixes are best-effort while the project is in active early development.

| Version | Supported |
| --- | --- |
| `v0.0.4` | Yes |
| `v0.0.3` | Best effort |
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
