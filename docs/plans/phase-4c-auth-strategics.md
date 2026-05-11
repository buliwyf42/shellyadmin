# Phase 4c — Auth Strategics (T1 TOTP + T3 PATs)

> **Status**: planned. Targeted release: v0.3.0 (parallel to 4b).
> **Aggregate aufwand**: 4–6 working sessions across 2 sub-blocks.
> **Risk**: high (auth surface; user-facing change with no rollback path once enabled).

Two independent auth additions that strengthen the operator-facing
gates beyond the v0.2.11 server-side session store. Either can ship
without the other; both depend on the existing session store from S5.

---

## Block 4c.1 — T1: TOTP 2FA

**Aufwand**: 3–4 sessions. **Risk**: high.

### Why

Single-factor login is the residual cookie-theft risk after S5. A
stolen cookie is revocable via Logout, but the operator's password
+ username pair leak (browser-saved-passwords scrape, keylogger,
post-it on the monitor) bypasses revocation. TOTP closes that
window: even with a stolen credential pair, the attacker needs the
2FA secret to mint a fresh session.

The threat model in ADR-0012 explicitly listed 2FA as the
"recommended Phase 4 lift" once server-side sessions are in place.

### Target shape

**DB / models:**

```sql
CREATE TABLE totp_state (
    username TEXT PRIMARY KEY,
    secret_cipher TEXT NOT NULL,     -- secretbox-sealed base32 TOTP secret
    enrolled_at TEXT NOT NULL,
    last_verified_at TEXT NOT NULL DEFAULT '',
    backup_codes_cipher TEXT NOT NULL,  -- secretbox-sealed JSON [hashed]
    backup_codes_used INTEGER NOT NULL DEFAULT 0
);
```

```go
type TOTPState struct {
    Username         string
    Enrolled         bool
    EnrolledAt       string
    LastVerifiedAt   string
    BackupCodesIssued int
    BackupCodesUsed   int
}
```

**Settings:**

```go
type AppSettings struct {
    // ...
    TOTPRequired bool `json:"totp_required,omitempty"`
}
```

When TOTPRequired = true AND the user is enrolled, login refuses
password-only auth.

**Endpoints (new):**

- `POST /api/totp/enroll` — body `{}`. Generates a fresh secret,
  stores it under `pending_secret_cipher` in a session-scoped
  cookie field (NOT yet committed to DB). Returns
  `{ secret, qr_uri, recovery_codes }`. The operator scans the
  QR into their TOTP app.
- `POST /api/totp/verify-enroll` — body `{ "code": "123456" }`.
  Confirms the TOTP code; commits the secret + backup codes to
  the DB and clears the pending state.
- `POST /api/totp/disable` — body `{ "code": "..." }`.
  Verifies one valid TOTP (or backup code), deletes the row.
- `GET /api/totp/status` — returns the operator's enrolment
  state. Used by Settings to render the "set up 2FA" / "disable
  2FA" button.

**Login change:**

`POST /api/login` body grows an optional `totp_code` field. The
handler flow:

1. Password verify (same as today).
2. If TOTP row exists for the user, require `totp_code`.
   - Empty → return 401 with `{"error": "totp_required"}` so the
     SPA can show a second-step prompt.
   - Wrong → 401 same shape; increments the existing
     login_state counter (the lockout already covers this).
3. On success, issue session as today.

### Approach (sequenced)

1. **Migration + DB methods** (0.5 session). New `028_totp_state.sql`
   migration; `db.TOTPState`, `db.GetTOTP`, `db.SetTOTP`,
   `db.DeleteTOTP` methods + Store interface entries.
2. **TOTP code logic** (0.5 session). `internal/services/totp.go`
   using `github.com/pquerna/otp` (or the lighter-weight HOTP/TOTP
   stdlib-compatible impl `xlzd/gotp`). Choose one library; pin
   minor.
3. **Enrollment endpoint pair** (1 session). The two-call
   enrol/verify-enrol dance avoids storing an unverified secret.
4. **Login integration** (1 session). Modify Login handler to
   require TOTP when enrolled.
5. **SPA — Settings page TOTP card** (1 session). QR code render
   (canvas or `qrcode-svg`), backup codes list, enable/disable
   buttons.
6. **SPA — Login page second-step prompt** (0.5 session).

### Acceptance

- Operator can enrol, log out, log back in with username+password+TOTP.
- Operator can use a backup code if their TOTP device is lost; the
  used code marks itself used and cannot be reused.
- `TOTPRequired = true` blocks password-only login for the enrolled
  user. Non-enrolled users can still log in password-only (escape
  hatch for the first-time enrolment flow).
- Existing Cookie + server-session flow unchanged.
- Account-lockout (Q20 from Phase 1) still counts TOTP failures
  against the per-user budget.
- E2E test (Playwright or curl-script): enrol → logout → login
  with code → logout → login with backup code → bad code rejected.

### Risks + mitigations

| Risk | Mitigation |
|---|---|
| Operator loses TOTP device + all backup codes | Document the recovery path: `shellyctl totp-reset <user>` subcommand that requires direct DB access (the lockout against this is the operator's filesystem perms on `/data/shellyctl.db`). |
| Clock skew between server + TOTP device | otp lib's `Validate` accepts +/- 1 step (30s) by default; document the tolerance. |
| Backup codes leak in audit log | SanitizeLogMessage already redacts; add a test case for the TOTP-enrolment audit row. |
| TOTP secret stored next to DB (same Volume Exfil threat as v0.2.x credentials) | secretbox-sealed; ADR-0013 (encryption-key externalisation, v0.3.0 hard-fail) closes this. |

### Out of scope

- WebAuthn / passkeys (T2 from Phase 4 strategic; XXL; depends on T1).
- SMS / email second factor — well-known weaker than TOTP; not worth
  the SMS-relay setup.

---

## Block 4c.2 — T3: Personal Access Tokens (PATs)

**Aufwand**: 2 sessions. **Risk**: medium.

### Why

Programmatic API consumers (Home Assistant, scripts, cron jobs) today
have to fake the cookie+CSRF dance. MCP has its own token; the rest
of the SPA-API surface doesn't. A PAT is a long-lived, revocable,
scope-tagged bearer token suitable for headless callers.

This is also the precondition for moving any /api/* mutation into a
machine-authenticable flow without weakening the human session model
(passwords + TOTP + lockout stay session-only).

### Target shape

**DB:**

```sql
CREATE TABLE personal_access_tokens (
    id TEXT PRIMARY KEY,            -- public id, "pat_" prefix
    username TEXT NOT NULL,
    name TEXT NOT NULL,             -- operator-supplied label
    token_hash TEXT NOT NULL,       -- argon2id PHC of the bearer token
    scopes TEXT NOT NULL,           -- JSON array of scope strings
    created_at TEXT NOT NULL,
    last_used_at TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL DEFAULT '',  -- '' = never
    revoked_at TEXT NOT NULL DEFAULT ''
);
```

The plaintext token is shown to the operator ONCE at creation; only
its hash is stored. Format: `pat_<id>_<random>` where `<id>` matches
the `id` column (for fast lookup) and `<random>` is the
cryptographic component (32 bytes hex).

**Endpoints:**

- `POST /api/tokens` — body `{ "name": "...", "scopes": [...],
  "expires_in_days": int }`. Returns `{ "id": "pat_...",
  "token": "pat_..._...", "expires_at": "..." }`. Token shown once.
- `GET /api/tokens` — list (no plaintext, just metadata).
- `DELETE /api/tokens/:id` — revoke.

**Middleware:**

`internal/middleware/auth.go` extends RequireAuth: if a request
carries `Authorization: Bearer pat_*`, look up by id, verify hash,
check `revoked_at == "" AND expires_at IS empty OR > now()`,
populate the gin context with `username + scopes` from the row, and
proceed. The existing cookie/session path is unchanged.

**Scopes (initial set):**

- `devices:read` — `/api/devices*` GET only.
- `devices:write` — refresh, forget, bulk_action.
- `firmware:read` — firmware/status, install/status.
- `firmware:write` — firmware/check, firmware/update.
- `provision` — provision + upload_user_ca.
- `settings:read`, `settings:write`.
- `admin` — all of the above.

CSRF requirement is **dropped** for PAT-auth'd requests — the bearer
token IS the proof-of-intent. RequireCSRF middleware checks for the
Authorization header and skips the nonce comparison.

### Approach

1. **Migration + DB methods** (0.5 session). `029_personal_access_tokens.sql`
   + `db.PAT`, `db.CreatePAT`, `db.GetPAT`, `db.ListPATs`,
   `db.RevokePAT`.
2. **Service layer** (0.5 session). `internal/services/tokens.go`
   handles the create/list/revoke surface plus the
   `LookupPAT(token string) (PAT, error)` used by middleware.
3. **Middleware integration** (0.5 session). RequireAuth and
   RequireCSRF both gain a "is this a PAT request?" early-exit;
   scope check is left to the per-handler validation (a new helper
   `RequireScope(scope string) gin.HandlerFunc`).
4. **SPA — Settings → API Tokens card** (0.5 session). List, create,
   revoke. The plaintext token is shown in a one-time alert with a
   "copy to clipboard" button.

### Acceptance

- Operator can mint a PAT with scope `devices:read`, use it from
  curl: `curl -H "Authorization: Bearer pat_..." /api/devices`
  returns 200.
- Same PAT can NOT call `/api/bulk` (returns 403 scope-violation).
- Revoked PAT returns 401 within the next request.
- Login attempts using a PAT in the cookie field still go through
  the normal session-cookie pipeline (no confusion path).
- CSRF token NOT required for PAT-auth'd POST/DELETE.

### Risks + mitigations

| Risk | Mitigation |
|---|---|
| PAT leaked to a forwarded log line | SanitizeLogMessage covers Authorization headers; add a test case. |
| Token enumeration via id prefix | id is a random component too (`pat_<8 hex>_<32 hex>`); lookup is constant-time per id; absence returns the same timing as a malformed token. |
| PAT bypasses TOTP — counterintuitive | Document it: PATs are a separate auth path with their own revocation; TOTP gates the human session, PATs gate the machine session. Both pass through the lockout counter when used in failure. |

---

## Cross-block sequencing

Two paths, both independent:

```
T1 (TOTP)  ─── ships first because it closes the password-theft window
                that the LAN-lateral threat model in ADR-0012 puts highest.
T3 (PATs)  ─── ships either before or after; no dependency between
                the two.
```

Both depend on the existing server-side session store (S5 in v0.2.11)
and the account-lockout pipeline (Q20 in v0.2.10).

Recommended PR order:

1. T1 enrol + verify-enrol endpoint pair.
2. T1 login integration + SPA prompt.
3. T1 settings UI card.
4. T3 PAT CRUD endpoints.
5. T3 middleware integration.
6. T3 SPA tokens UI card.
