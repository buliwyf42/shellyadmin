# ADR-0015: Single-Instance-Only Operational Constraint

- Status: `Accepted`
- Date: 2026-05-11
- Implements: post-v0.2.12 follow-up (Phase 4 / T10 from the consolidated review)
- Related: ADR-0001 (Product Scope and Non-Goals), ADR-0011 (MCP Server)

## Context

ShellyAdmin runs as a single container. Three pieces of state live
in-process and are NOT replicated across instances:

1. **Login rate-limit map** (`internal/middleware/ratelimit.go`) —
   in-memory `map[string][]time.Time` per IP for login attempts and
   per-IP for API rate-limit. Two instances would split the count
   and let an attacker hit each from a different ingress.
2. **MCP listener controller** (`internal/services/app_mcp.go`) —
   the live `*http.Server` plus its lifecycle mutex. A second
   instance reading the same SQLite would try to bind the same
   `:8081`, contend on the settings row's `__retention_bypass`
   flag, and emit duplicate `mode=confirmed` audit rows for the
   same MCP request.
3. **Background workers** — firmware-check scheduler, audit
   retention pruner, auto-backup job, session sweeper. Each is a
   single goroutine; two instances would issue duplicate scheduled
   work (`Shelly.CheckForUpdate` calls double-billed against the
   target's auth-lockout budget, two PruneAuditLogOlderThan
   transactions racing each other).

SQLite-WAL handles concurrent readers + one writer in the same
process; it does *not* handle two writer processes on the same file
robustly. WAL mode mitigates some lock contention but doesn't
prevent double-spawn of background jobs.

The consolidated review flagged this as "the architecture decision
that will behind future growth" — accurate, but only if growth
means horizontal scaling. The product scope (ADR-0001: single
operator, trusted LAN) gives no reason to scale horizontally; the
load is single-digit RPS at most.

## Decision

**ShellyAdmin is single-instance-only. Running two containers
against the same SQLite database is unsupported and will be
detected at startup.**

### Detection (v0.3.0)

On startup, the service writes a `LOCK` row to a new
`runtime_locks` SQLite table:

```sql
CREATE TABLE runtime_locks (
    key TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL,
    acquired_at TEXT NOT NULL,
    pid INTEGER NOT NULL,
    hostname TEXT NOT NULL
);
```

The boot sequence does:

1. Read `runtime_locks` WHERE key='primary'. If a row exists AND its
   `acquired_at` is fresher than 5 × the heartbeat interval (~5
   minutes), log an error and exit non-zero. The operator gets a
   message naming the other instance's hostname/pid.
2. Otherwise INSERT (or UPDATE) the row with a fresh instance_id,
   pid, hostname, and now() timestamp.
3. Start a heartbeat goroutine that updates `acquired_at` every 60
   seconds.
4. On graceful shutdown, DELETE the row.

A stale row (5+ minutes without heartbeat) is overwritten on the
next startup — covers the case where a previous container was
killed and the deletion never ran. Operators bringing a hung
instance up after a power-fail don't need to manually unlock.

### Why Not Multi-Instance

The four components that would need to change to support
multi-instance:

- **Rate-limit map** → Redis (or PostgreSQL with a TTL extension).
- **MCP controller** → leader election (Raft / etcd / Postgres advisory locks).
- **Background workers** → leader-only execution (only the leader runs
  the firmware-check scheduler, etc.) OR distributed locks.
- **SQLite** → PostgreSQL or MySQL.

That stack is a substantial product change for a tool whose entire
use case fits comfortably in a single 50MB container talking to a
fleet of 200 devices. The consolidated review estimated the
total-effort delta at "1-2 quarters of focused work" — not paid
back unless ShellyAdmin grows beyond its current product scope.

If a future product-scope shift makes multi-instance necessary,
this ADR is the place to revisit and supersede. Until then,
`runtime_locks` is the explicit door-closer.

## Consequences

**Positive**

- Operators get a clear error message instead of subtle bugs
  (duplicate firmware checks, lock-table contention, occasional
  500s when both instances try to write the same audit row at
  the same microsecond).
- The architecture decision is documented; a future contributor
  has a place to start when the question comes up.
- The `runtime_locks` table is reusable for other invariants —
  e.g. "only one firmware-install job in flight per device" could
  use the same row-with-heartbeat pattern.

**Negative**

- Operators rolling a new container by stopping the old one and
  starting the new one without `docker compose down` first will hit
  a startup error for up to 5 minutes (the heartbeat-staleness
  window). The error message includes the other instance's pid +
  hostname so the operator can resolve it manually
  (`docker stop <name>` or wait for the heartbeat to time out).
- A power-fail at the wrong moment (heartbeat thread alive but
  not the rest of the service) is impossible in practice — the
  heartbeat shares the service's main goroutine context — but if
  a future refactor splits them, the staleness threshold may need
  to shrink.

**Mitigations**

- Document the staleness window prominently in DEPLOYMENT.md.
- Provide a `shellyctl unlock` subcommand for the rare manual-
  recovery case (planned for v0.3.x).

## Related Work

- ADR-0011 §"Reconcile-Lifecycle" (the MCP controller's own mutex
  is a per-instance concern; ADR-0015 is the cross-instance
  layer).
- DEPLOYMENT.md "Pre-deploy snapshot" section — pairs with
  runtime_locks for the recreate workflow.
