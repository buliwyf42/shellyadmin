# ADR-0017: First-Run Setup (operator login in the database)

- Status: `Accepted`
- Date: 2026-05-20
- Related: ADR-0013 (Encryption Key Externalization), ADR-0015 (Single-Instance Constraint)

## Context

Through v0.3.x the operator login came exclusively from environment
variables: `SHELLYADMIN_USER` (default `admin`) and
`SHELLYADMIN_PASS_HASH` (an argon2id PHC string from
`shellyctl hash-password`). A missing hash **panicked at startup**.
Both values were frozen into `api.Config` (`User`/`PassHash`) and
compared directly in the login handler.

This forced every operator to run `shellyctl hash-password` and hand-edit
an `.env` before the UI would even start — a poor first-run experience for
a homelab tool. We want a fresh instance to boot, present a setup screen,
and let the operator create the admin account from the browser.

## Decision

The operator login moves into the database as the single source of truth.

- **Storage**: a dedicated single-row table `admin_credentials`
  (migration 031), holding `username` + `pass_hash` (argon2id PHC) +
  `updated_at`. Deliberately NOT part of `AppSettings`, which round-trips
  to the SPA via `GET /api/settings` — a password hash must never reach
  that surface. The hash is one-way, so it is stored verbatim (not
  secretbox-sealed) and shares the `shellyctl.db` backup/rollback
  boundary like every other row.
- **Resolution**: the login handler resolves the credential at request
  time via `AppService.AdminCredential()` (DB-backed). A `cfg`-based
  fallback remains for handler tests that seed `Config` rather than the DB.
- **Boot**: the startup panic is gone. If no credential is configured the
  server boots into **setup mode** — it serves only the setup screen and
  the public status probe; every authenticated route is naturally
  unreachable because no login can succeed.
- **Env migration**: `ImportEnvCredentialOnce` imports a still-present
  `SHELLYADMIN_PASS_HASH` into the DB exactly once (only when no DB
  credential exists). Existing deployments upgrade seamlessly; afterward
  the env var is irrelevant.
- **Endpoints**:
  - `GET /api/setup/status` — public, returns `{configured: bool}` only.
  - `POST /api/setup` — public, rate-limited, one-shot (409 once
    configured). The single unauthenticated mutation in the app.
  - `POST /api/account/credentials` — authenticated + **cookie-only**
    (a PAT must not be able to rotate the login that gates it, same
    privilege-escalation guard as token management). Verifies the current
    password, then updates and revokes all sessions.
- **Recovery**: `shellyctl reset-auth --force` clears the row, returning
  the server to setup mode on the next boot. This is the supported
  forgotten-password path now that the env hash is no longer authoritative
  — mirrors `shellyctl unlock --force`.

### Setup-endpoint race safety

`POST /api/setup` is the only unauthenticated write, so it must not be
exploitable into overwriting an account. Two layers guard it: the handler
rejects when `adminCredential()` already reports configured, and
`SetupAdminCredential` re-checks under a process-local mutex before
writing (check-then-insert). Cross-process races are already excluded by
the single-instance runtime lock (ADR-0015).

## Consequences

- The encryption key requirement (ADR-0013) is unchanged and still
  enforced at boot — sessions and TOTP secrets are sealed with it, so
  setup mode still requires `SHELLYADMIN_ENCRYPTION_KEY`.
- `SHELLYADMIN_PASS_HASH` / `SHELLYADMIN_USER` are demoted from
  "required credential" to "optional one-time import seed". Documentation
  and deploy artifacts should stop presenting the hash as mandatory.
- Operators can change their username/password from Settings without a
  redeploy; the change revokes existing sessions and forces re-login.
