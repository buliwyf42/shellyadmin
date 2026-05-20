# Security Model

## Intended Environment

ShellyAdmin is designed for:

- trusted LAN environments
- a single trusted operator
- local network device administration

It is not designed for:

- direct internet exposure
- multi-tenant use
- public API access

## Authentication

- single admin login
- session-cookie authentication
- login rate limiting
- POST logout

### Admin credential storage

- Since **v0.4.0 (ADR-0017)** the operator login lives in the database
  (single-row `admin_credentials` table), not the environment. A fresh
  instance boots into **first-run setup** (`/setup`) where the admin account
  is created in the web UI; the stored value is an argon2id PHC hash,
  constant-time compared on login. No startup panic when unset — the server
  serves only the setup screen until configured.
- `SHELLYADMIN_PASS_HASH` (or `_FILE`) is now **optional**: if set on a fresh
  instance it is imported into the database once at boot (seamless upgrade),
  then ignored. Generate with `shellyctl hash-password <plaintext>`.
- Change the login later in Settings → "Operator Account". Recover a forgotten
  password with `shellyctl reset-auth --force` (clears the row → next boot is
  setup mode again).
- Removed in v0.2.0: the deprecated plaintext `SHELLYADMIN_PASS` env var.
  v0.0.15 added `_HASH` and started warning on plaintext use; v0.2.0 closed
  the deprecation window.

## Session Security

- signed cookies
- `Secure` cookies supported for proxied TLS deployments
- sessions should be considered LAN-admin sessions, not internet-facing auth tokens

## Request Handling

- request size limits are enforced on JSON endpoints
- mutating operations require authenticated session access
- delete actions require explicit confirmation in UI
- preview coverage for risky actions is still being expanded

## Logging

Operator-facing logging is audit events in SQLite:

- stored in SQLite
- meaningful actions only
- intended as the in-app log surface

Secrets should never be written to logs in raw form.

## Provisioning Secrets

Current product decision:

- templates may contain real secrets
- auth groups may contain real secrets (`password` and/or `ha1`)

Required safeguards:

- secret-bearing templates should be clearly marked in the UI
- secret-bearing auth groups should be clearly marked in the UI
- exports should redact secret values by default
- password-derived device auth material should remain ephemeral during execution

### Encryption at Rest

Device credential `password` and `ha1` values stored in `credentials` and
`credential_groups` are encrypted with XSalsa20-Poly1305 (NaCl secretbox). The
32-byte key is resolved in this order at startup:

1. `SHELLYADMIN_ENCRYPTION_KEY` — base64-encoded 32-byte key. Also honours the
   `_FILE` suffix (reads the key from a file path).
2. `${DATA_DIR}/shellyadmin.key` — generated on first boot if nothing else is
   configured. The file is written `0600`.

The key never leaves process memory at runtime; the SQLite file alone is not
enough to recover credentials. **Back up the key file alongside the database**
— losing the key permanently orphans every stored credential.

This defends against offline DB exposure (stolen backup, container escape
reading `/data`, misconfigured volume). It does not defend against an attacker
with live access to the running process.

## Network Scope

- scans may target any user-entered CIDR
- subnet-size and operational safeguards should still exist
- provisioning targets are validated as IP addresses and restricted to local/private/link-local ranges
- loopback, unspecified, and multicast targets are rejected for provisioning

## Operational Safety

- scan and refresh workflows use bounded worker pools
- concurrency limits cap active probes instead of spawning one goroutine per target

## Container Security

Recommended runtime posture:

- non-root user
- read-only root filesystem
- persistent writable data volume only where needed
- dropped Linux capabilities
- no-new-privileges

## TLS

The app itself does not need to terminate TLS.

Recommended pattern:

- optional reverse proxy terminates TLS
- app remains private and internal

## Network Segmentation (M13)

ShellyAdmin's threat model assumes the LAN is hostile (ADR-0012 from
the consolidated review, post-v0.2.10). A compromised IoT device, a
visitor's phone on guest Wi-Fi, or a vulnerable smart appliance on
the same flat subnet is the highest-probability initial-access
vector. Operators are expected to mitigate at the network layer:

**Minimum recommended segmentation:**

- **Admin VLAN** — the operator workstation reaching the ShellyAdmin
  SPA at `:8100`. Restricted, MFA-protected if possible.
- **IoT VLAN** — Shelly devices, smart speakers, TVs, anything that
  speaks an unauthenticated LAN protocol. Egress to the Admin VLAN
  is blocked at the firewall (`deny inter-vlan` for source=IoT,
  dest=Admin). The ShellyAdmin container's outbound rules allow it
  to reach this VLAN; the reverse path is closed.
- **Guest VLAN** — visitor devices. No reach to Admin or IoT.

**Per-firewall examples:**

- *OPNsense*: separate interfaces per VLAN, "block from IoT_net to
  Admin_net" firewall rule before the default-allow rule.
- *UniFi*: traffic rules in the "LAN In" pipeline; assign each VLAN
  to its own network with `Inter-VLAN routing: disabled`.
- *Pure-Linux router*: `iptables -A FORWARD -i iot0 -o admin0 -j DROP`
  before the FORWARD-ACCEPT.

**Reverse-proxy posture:**

When ShellyAdmin sits behind a reverse proxy (typical homelab),
configure:

- TLS termination on the proxy (`COOKIE_SECURE=true` in compose).
- Operator-IP allowlist on the proxy if the SPA is reachable from
  outside the Admin VLAN.
- `SHELLYADMIN_TRUSTED_PROXIES=<proxy CIDR>` so the binary's
  `ClientIP()` reads `X-Forwarded-For` correctly (per-IP rate-
  limit accounting depends on this).

## Off-Host Log Forwarding (M12)

The audit log is persisted in `audit_log` with the v0.2.11 hash
chain + append-only trigger. Tamper-evidence is on; tamper-
prevention against an attacker with filesystem access is not.
Forward logs off-host for a true append-only audit trail.

**Container stdout → Loki via Promtail:**

```yaml
# promtail-config.yaml fragment — pick up shellyadmin's structured
# slog output that v0.2.11 tees to stderr.
scrape_configs:
  - job_name: shellyadmin
    docker_sd_configs:
      - host: unix:///var/run/docker.sock
        filters:
          - name: name
            values: [shellyadmin]
    relabel_configs:
      - source_labels: [__meta_docker_container_name]
        target_label: container
    pipeline_stages:
      - json:
          expressions:
            level: level
            request_id: request_id
            risk_level: risk_level
            msg: msg
      - labels:
          level:
          risk_level:
```

**Useful Grafana panels / Loki alerts:**

- `count_over_time({container="shellyadmin"} |~ "login blocked: account locked" [1h])`
  — alert on >0 means brute-force attempt cleared rate-limit.
- `count_over_time({container="shellyadmin"} | json | risk_level="high" [24h])`
  — daily count of high-risk MCP/API actions; spike = investigate.
- `{container="shellyadmin"} | json | request_id="<id>"`
  — full forensic trace by correlation ID, paired with the
  matching `audit_log` row in SQLite.

The same approach works with Splunk, ELK, or any sink that consumes
Docker stdout JSON.

## Explicit Constraints

The current design intentionally does not include:

- multi-user RBAC
- external API tokens
- public internet hardening beyond sensible defaults for a LAN appliance
