# ADR-0012: STRIDE Threat Model + Defense-in-Depth Posture

- Status: `Accepted`
- Date: 2026-05-11
- Implements: v0.2.10 (Phase 1 hardening) + Phase 2 follow-ups
- Roadmap link: [docs/roadmap.md](../roadmap.md)
- Related: ADR-0001 (Product Scope and Non-Goals), ADR-0003 (Device Auth and Credentials), ADR-0011 (MCP Server)

## Context

ADR-0001 set the product scope as "single-operator, trusted-LAN admin
tool". The threat model has lived implicitly in SECURITY.md prose
since. A consolidated architecture + security review (May 2026)
identified three systemic risks that were previously absorbed by the
"trusted-LAN" assumption:

1. A compromised IoT device on the same subnet (Gäste-WLAN crossover,
   vulnerable smart TV, etc.) is a realistic adversary, not a thought
   experiment. Treating LAN as "trusted" assumed away the most
   probable attack origin in a homelab.
2. Supply-chain compromise of the GHCR image was the worst-case
   single-event scenario — Operator pulls `:latest`, recreate, full
   container compromise — but Phase-1's defense was only `provenance:
   mode=max` + SBOM, with no operator-side verification path.
3. The cookie session was clientside-encrypted, valid 7d, and the
   CSRF token rode every authenticated response header. One DOM-
   injection sink in the SPA was a complete CSRF bypass.

The operator decision (consolidated-review handoff) was to abandon
the trusted-LAN assumption while keeping ADR-0001's single-operator
scope intact. This ADR formalizes that posture so a future
contributor reading the codebase cold can see what each defense
layer covers and what is explicitly out of scope.

## Decision

### Trust Boundaries

| Boundary | Direction | Enforcement | Notes |
|---|---|---|---|
| Internet ↔ Reverse Proxy | inbound | Operator-external TLS + optional auth | Out-of-scope for in-binary code; SECURITY.md documents the expected fronting |
| LAN ↔ HTTP `:8080` | inbound | Session cookie + CSRF + Rate-Limit + Account-Lockout | Layer-3-reachable from compromised LAN host. Assumed-hostile. |
| LAN ↔ MCP `:8081` | inbound | Bearer/URL token, constant-time, RateLimit (S8) | Bind defaults to `127.0.0.1` since v0.2.10. |
| Container ↔ Shelly devices | outbound | per-device Digest auth, optional TLS validation | Egress-allowlist gate planned in M5. |
| Container ↔ stdio-MCP child | parent-process | filesystem perms on `/data/shellyctl.db` | No transport auth; documented in ADR-0011. |
| Database file ↔ Crypto | seal/open | NaCl secretbox + key file | Phase-2 S6 enforces external key (no auto-generation). |

### STRIDE Mapping (Threat → Defense)

| Category | Threat | Phase 1 / 2 Defense | Phase 3+ Plan |
|---|---|---|---|
| **Spoofing** | Stolen admin cookie used after operator-detected breach | Server-side session store with revocation (S5, Phase 2) | TOTP 2FA (T1, Phase 4) |
| **Spoofing** | Forged session cookie from leaked SHELLYADMIN_SECRET | Cookie HMAC + Secret rotation triggers session invalidation | WebAuthn passkeys (T2) |
| **Tampering** | Audit-log deletion by post-compromise admin | Hash-chained audit_log + SQLite append-only trigger (S2) | Off-host webhook forwarder (T11) |
| **Tampering** | Direct DB row injection via compromised volume | NaCl secretbox at-rest for credentials + MCP token; FK enforcement | External key (S6) raises bar against full-volume exfil |
| **Repudiation** | Admin denies an action | Request-ID correlation, risk_level on action rows, MCP preview/confirm pair | Hash-chain integrity check tool |
| **Information Disclosure** | MCP token in browser history / proxy logs (URL-path form) | Header form preferred (documented); token-format regex blocks `/` (Q14) | Documentation in ADR-0011 caveat |
| **Information Disclosure** | XSS-grabbed CSRF token | CSRF no longer echoed as response header (Q12); SameSite=Strict (Q21) | Trusted-Types CSP (S17), no `unsafe-inline` (M6) |
| **Information Disclosure** | Volume-snapshot exfil reveals both DB and encryption key | External key requirement (S6) — operator must store key separately | HSM/PKCS11 slot (T4) |
| **Denial of Service** | Login brute-force | Argon2 cost + LoginRateLimit + AccountLockout (Q20) | TOTP raises bar further |
| **Denial of Service** | audit_log fill consumes disk | Retention job (S1) | Off-host forward (T11) |
| **Denial of Service** | MCP token stolen, unbounded RPS | MCP rate-limit middleware (S8) | — |
| **Elevation of Privilege** | Compromised image → container-root | Container hardening (read-only FS, cap_drop ALL, no-new-privileges, USER shelly in S14) + Cosign sign+verify (S3 + operator-side gate) | SLSA attestation chain |
| **Elevation of Privilege** | Compromised admin → MCP-rotate-token → physical-actor control | Confirm-gate on every state-changing MCP tool (ADR-0011) | MCP-agent threat model (T5) |

### What This ADR Does NOT Try to Cover

ADR-0001's non-goals stand. In particular this threat model does NOT
assume:

- Multi-tenant isolation (single operator role only — see ADR-0001).
- Authenticated network peers (Shelly devices use Digest auth at most;
  ShellyAdmin defaults to plaintext HTTP on the LAN, optional TLS).
- Sandboxed plugins / extension surface — there is no plugin model.
- Compliance certifications (SOC2 / ISO27001 / DSGVO are out-of-scope;
  no personal data is stored beyond the operator's chosen username).
- Anti-forensics by a Kernel-level rootkit on the host — the threat
  model assumes the host kernel + container runtime are intact.

### What an Operator Must Still Do

The product-side defenses are necessary but not sufficient. An operator
running ShellyAdmin against the threat model documented here is
expected to:

1. **Network-segment** the admin VLAN from IoT / Gäste-WLAN (M13 doc
   to land in Phase 3).
2. **Verify image signatures** with `cosign verify` before recreate
   (S3 step + DEPLOYMENT.md instruction). A signed image that the
   operator does not verify is "theater".
3. **Externalize the encryption key** (Phase 2 S6 makes this mandatory
   from v0.3.0 onwards; v0.2.10 emits a stderr deprecation warning).
4. **Back up the SQLite database AND key separately** — see ADR-0006.
5. **Front the binary with TLS-terminating reverse proxy** for
   non-loopback access; the binary itself speaks plain HTTP.

## Consequences

**Positive**

- Threat model is no longer prose-only — future-you (or a code
  reviewer) can grep ADR-0012 to see whether a proposed defense is
  in-scope or out-of-scope.
- Defense layers each have a named threat — easier to evaluate
  whether a future change reduces or expands the surface.
- Phase 1 ↔ Phase 2 ↔ Phase 4 sequence has a documentation anchor.

**Negative**

- A STRIDE table looks deceptively complete. Real-world threats
  rarely partition cleanly into Microsoft's six categories; the
  table is a *checklist* against systematic blindness, not a proof
  of coverage.
- The ADR locks in the trusted-LAN-is-hostile assumption; revisiting
  that decision (e.g. to skip 2FA on a closed homelab) requires
  re-opening this ADR rather than a code-level decision.

**Mitigations**

- Re-review on every Phase boundary. If a measured incident or a
  community pen-test (T9, Phase 4) shows a class of attack that
  doesn't fit cleanly into one of the rows above, the table needs
  to expand.
- Anchor Phase 4's "external pen-test" task (T9) to a re-check of
  this ADR's coverage.

## Notes on Defense Sequencing

The Phase-1 → Phase-2 → Phase-4 ordering is **not** STRIDE-priority
ordering — it is **risk-reduction-per-effort** ordering from the
consolidated review:

1. Quick-wins first (Phase 1): supply-chain SHA-pins, CSRF header
   removal, account-lockout — high blast-radius, low engineering
   cost.
2. Structural hardening (Phase 2): server-side sessions, audit
   hash-chain, encryption-key externalization — high blast-radius,
   medium-high engineering cost (1–3 days each).
3. Strategic (Phase 4): 2FA, external pen-test, MCP-agent threat
   model — requires earlier phases as substrate.

A future contributor opening a security PR should consult this ADR's
STRIDE table to place the new defense in context, then check the
consolidated-review plan (in the plans/ archive) for the phase
sequence rationale.
