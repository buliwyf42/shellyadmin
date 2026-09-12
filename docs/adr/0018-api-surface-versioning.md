# ADR-0018: The Stable API Surface Is Unversioned `/api/*` (T7)

- Status: `Accepted`
- Date: 2026-09-12 (accepted at the v1.0.0 cut)

## Context

[ARCHITECTURE.md § API Versioning Policy](../ARCHITECTURE.md) states that
once v1.0 lands, "`/api/v1/*` stays stable" and breaking changes ship
under `/api/v2/`. **That prefix does not exist.** Measured 2026-09-12:
the router serves **52 routes under an unversioned `/api/*`**; the only
`v1` in the tree is the OpenAPI document's own path
(`/api/openapi/v1.json`) and the `info.version` field inside it.

While the project is pre-1.0 the mismatch is harmless — nothing is
promised. At the v1.0.0 cut the policy becomes binding, and a stability
guarantee whose subject is fictional is worse than no guarantee: the
first operator to pin a script against the documented `/api/v1/devices`
gets a 404.

So T7 — carried since the consolidated review as "concrete `/api/v1/*`
prefix mounting + the `Deprecation` header, queued for the v0.3 → v1.0
cut" — has to be resolved before 1.0, in one direction or the other.
The [v1.0.0 release cut plan](../plans/v1.0.0-release-cut.md) treats it
as the release's single architectural decision.

Two options were on the table:

1. **Mount the prefix.** Move all 52 routes to `/api/v1/*`, keep
   `/api/*` as an alias for one release line behind a
   `Deprecation: true` header, update the SPA client, `shellyctl`, and
   the OpenAPI `paths` keyspace.
2. **Declare the unversioned prefix stable.** Leave the routes where
   they are and make the policy describe them.

## Decision

**`/api/*` is the stable, versioned-by-generation-1 surface.** No
`/api/v1/*` prefix is introduced, now or retroactively.

The versioning policy applies unchanged in substance, only re-anchored:

- **`/api/*` stays stable from v1.0.0.** Routes may gain new optional
  query parameters and new optional response fields; existing required
  fields cannot rename or change shape. Adding a route is
  non-breaking; renaming a field or adding a required one is breaking.
- **A breaking generation ships under `/api/v2/*`.** The `/api/*`
  routes keep working for one full release line, marked with a
  `Deprecation: true` response header so the SPA, `shellyctl`, MCP
  clients, and ad-hoc scripts can warn before the window closes.
- **Removal**: an `/api/*` route may be deleted at the v3 cut, not
  before — two release lines of migration room, as previously written.
- **The OpenAPI document keeps `info.version: "v1"`** and its path
  `/api/openapi/v1.json`. The contract generation and the URL prefix
  are deliberately decoupled: the document describes generation 1, and
  generation 1 lives at `/api/*`. When `/api/v2/*` exists it gets
  `/api/openapi/v2.json`.

The `paths` keyspace of the OpenAPI document remains the contract
surface, and the `cmd/modelschema` drift check remains the mechanism
that catches accidental Go-struct changes tilting the wire shape.

## Rationale

The prefix buys a namespace, and the namespace is only worth its price
when a second generation is in sight. There is none: the roadmap
declares the feature set complete for the target deployment, and the
post-v0.4.0 triage already recorded T7 as "pure plumbing with no payoff
until a breaking `/api/v2` exists."

Every consumer of this API ships from this repository — the bundled
SPA, `shellyctl`, and the MCP tool surface — so a URL migration today
is a rename with no external beneficiary, plus a permanent alias to
carry for compatibility with operators' own scripts.

Crucially, **option 2 does not forfeit option 1's benefit.** A future
breaking generation can be introduced at `/api/v2/*` regardless of
where generation 1 sits; the prefix scheme starts working the moment it
is actually needed. What option 1 offered over option 2 was cosmetic
symmetry, paid for now against a need that may never arrive.

## Consequences

- **No code change.** Router, SPA client, CLI, OpenAPI document and all
  52 routes stay as they are. The 1.0 cut stays a documentation
  release, which was its premise.
- **The policy text in ARCHITECTURE.md must be re-anchored** to
  `/api/*` in the same change that accepts this ADR — leaving it
  describing `/api/v1/*` reproduces exactly the defect this ADR exists
  to close.
- **The surface is asymmetric on purpose**: `/api/*` for generation 1,
  `/api/v2/*` for generation 2. Anyone reading only the route table
  will find that odd; this ADR is the answer, and the policy section
  links here.
- **Retrofitting `/api/v1/*` later is off the table.** After 1.0 that
  move is breaking by the very policy this ADR sets, and it would have
  to travel as a full generation bump. The prefix decision is being
  spent here, not deferred again.
- **A second-generation cut costs slightly more** than it would with a
  symmetric prefix: the `Deprecation` header has to be attached to the
  unversioned group rather than to a `v1` group. That is a one-time
  cost inside a release that is already breaking.

## Related Work

- [ADR-0005](./0005-data-migrations-and-compatibility.md) — data and
  migration compatibility policy; the on-disk counterpart to this
  wire-format decision.
- [ADR-0001](./0001-product-scope-and-non-goals.md) — the
  single-operator trusted-LAN scope that makes "all clients ship from
  this repo" true, which is what removes the prefix's payoff.
- [v1.0.0 Release Cut Plan](../plans/v1.0.0-release-cut.md) — the
  release this decision gates; step 2 of its sequence.
