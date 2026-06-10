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
- optional TOTP 2FA with single-use backup codes (v0.3.0)
- personal access tokens (`pat_…`) with per-route scopes for headless
  callers; PAT-authed requests cannot mint, list, or revoke other PATs
  (v0.3.0)

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

Secrets stored in SQLite are encrypted with XSalsa20-Poly1305 (NaCl
secretbox). Sealed material includes:

- device credential `password` / `ha1` values (`credentials`,
  `credential_groups`)
- the MCP listener token persisted via Settings
- TOTP secrets and backup codes

The 32-byte key comes from `SHELLYADMIN_ENCRYPTION_KEY` (base64-encoded) or
`SHELLYADMIN_ENCRYPTION_KEY_FILE`. **Since v0.3.0 (ADR-0013) the key is
mandatory** — the server refuses to start without it. The pre-v0.3.0
auto-generated `${DATA_DIR}/shellyadmin.key` fallback was removed because a
key sitting next to the database shares its backup boundary: one stolen
volume snapshot defeated the encryption entirely. Upgraders: copy the legacy
file's contents to a secret store outside the data volume and point
`SHELLYADMIN_ENCRYPTION_KEY_FILE` at it (the startup error names the legacy
path when it detects one).

The key never leaves process memory at runtime; the SQLite file alone is not
enough to recover credentials. This defends against offline DB exposure
(stolen backup, container escape reading `/data`, misconfigured volume). It
does not defend against an attacker with live access to the running process.

#### Key backup

**Losing the key permanently orphans every sealed secret** — device
credentials, the MCP token, and TOTP enrollment. There is no recovery path.
Back up the key with the same rigor as the database, but in a **separate
location** (Docker secret, sops-encrypted file in a config repo, password
manager): keeping them in the same backup defeats the purpose of
externalizing the key.

#### Key rotation (`shellyctl rotate-key`)

`shellyctl rotate-key` re-encrypts every sealed value — device credentials,
credential groups, TOTP material, the persisted MCP token — from the
current key to a new one, in a single transaction. The admin login is
argon2id (not sealed) and unaffected.

Keys come from the environment, never argv (argv leaks into shell history
and `ps`): the current key from `SHELLYADMIN_ENCRYPTION_KEY[_FILE]` (the
same variables the server uses), the new key from
`SHELLYADMIN_NEW_ENCRYPTION_KEY[_FILE]`.

1. Stop the container. The command refuses to run while a server holds a
   fresh runtime-lock heartbeat — rotating under a live server would
   corrupt rows it writes concurrently.
2. Dry run (inside the container image or any host with the data dir):

   ```bash
   export SHELLYADMIN_NEW_ENCRYPTION_KEY="$(openssl rand -base64 32)"
   shellyctl rotate-key            # opens every blob with the old key, writes nothing
   ```

   A wrong current key fails here, before anything is touched.

3. `shellyctl rotate-key --force` — writes a timestamped DB backup
   (`shellyctl.db.backup-…`), then rotates. Any failure rolls the whole
   transaction back; the database is never left half-rotated.
4. Point `SHELLYADMIN_ENCRYPTION_KEY` / the key file at the **new** key and
   start the container. The old key no longer opens anything.
5. Verify a refresh against an auth-protected device succeeds, then retire
   the old key and the pre-rotation backup.

Rollback: restore the backup file together with the old key.

Background for older versions (pre-rotate-key): any sealed row left in the
database after a manual key swap surfaces as a `decrypt …` error in
whatever reads it — credential listing breaks, the Settings read path
breaks on a stale MCP token, and a TOTP-enrolled login can lock the
operator out. The export-with-secrets → clear-all-sealed-surfaces →
swap-key → re-import procedure documented in this section's git history
still works as a fallback, but the command supersedes it.

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

## MCP Listener Token Hygiene

The MCP listener (`:8081`, ADR-0011) accepts its token two ways:

1. `Authorization: Bearer <token>` header — **preferred**.
2. First URL path segment (`http://host:8081/<token>/`) — kept because
   `mcp-remote`-style clients and Home Assistant make header configuration
   awkward.

**The path form writes the token into anything that logs request paths**:
reverse-proxy access logs, log aggregators consuming container stdout,
browser history if the URL is ever opened interactively, and shell history
when pasted into `curl`. The token authenticates every MCP tool including
the confirm-gated state-changing ones, so treat those logs as
secret-bearing.

Guidance:

- Use the Bearer header wherever the client supports it.
- If the path form is unavoidable behind a reverse proxy, redact the first
  path segment in the proxy's access-log format, or disable access logging
  for the MCP port.
- **Rotate on suspected exposure**: save a new token in Settings → MCP —
  the listener reconciles live, no restart needed. Instances locked via the
  `SHELLYADMIN_MCP_TOKEN` env var ignore Settings; change the env var and
  restart instead.
- After rotation, every MCP client needs the new token; stale clients fail
  closed with 401.

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
- public internet hardening beyond sensible defaults for a LAN appliance
