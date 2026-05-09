# Development

## Tooling

Backend:

- Go 1.25+ (the floor moved from 1.24 → 1.25 in v0.1.16; gin v1.12.0
  pulls `quic-go/quic-go` for HTTP/3, which requires Go 1.25.0).
- `golangci-lint` v2.6+ (the v1 → v2 migration landed in v0.1.6; older
  binaries fail to load `.golangci.yml` because the schema changed).
  Install: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.0`.

Frontend:

- Node 20+

## Local Workflow

### Backend

```bash
make dev-backend
```

### Frontend

```bash
cd web
npm install
npm run dev
```

The Vite dev server listens on `5173` and proxies backend requests to `127.0.0.1:8080`:

- `/api`
- `/health`

Use the Vite URL for interactive frontend work:

```bash
http://<dev-host>:5173
```

Use the Go app directly on `8080` when testing embedded production assets.

For UI consistency checks, verify both:

- the Devices table `Last Success` column
- the per-device detail `Last Success` field

Both should follow the same locale-aware relative/absolute time policy from ADR-0007.

## Production Build

```bash
make build
```

The production-oriented path is the same one used for Docker builds:

- build the Svelte frontend
- sync it into `cmd/shellyctl/dist`
- compile the Go binary with embedded static assets

To rebuild only the frontend bundle that the Go app embeds:

```bash
make frontend
```

To sync frontend assets without reinstalling dependencies:

```bash
make frontend-sync
```

## Tests

### Backend

```bash
go test ./...
go vet ./...
golangci-lint run ./...
```

Coverage as of v0.1.18 (after the M3 testability foundation):

- `internal/core/firmware`: ~71% — JSON-RPC translation paths,
  `Schedule.*`-based auto-update, gen<2 short-circuits, RPC error
  sentinel handling. Shared httptest fixture `fakeShelly` lives in
  `internal/core/firmware/helpers_test.go` — reuse it for new firmware
  tests.
- `internal/core/scanner`: ~39% — JSON-RPC failure-handling branches
  and `LastSeen`/`AuthLockedUntil` clock contract via FakeClock. The
  CIDR / mDNS / `ScanSubnets` concurrency code is intentionally
  out-of-scope for unit tests.
- `internal/core/setters`: ~56% — payload-shape contracts, percent
  clamping, the 404/-32601 method-not-found path, and the `(bool, string)`
  / `(ok, supported, message)` returner shapes.
- `internal/core/provisioner`: ~62% — section-by-section coverage plus
  the multi-section integration smoke
  (`TestProvisionDevice_MultiSectionSmoke`) that pins
  `Shelly.SetAuth` HA1 = `SHA-256("admin:serial:pass")`.
- `internal/core/clock`: 100% — small enough to fully cover.

Services that orchestrate background jobs (`internal/services/app_jobs.go`,
`internal/services/app_backup.go`) have representative happy-path and failure
coverage. Add new tests alongside the existing files when you extend those
flows.

#### OnClient + Clock injection (testability seams)

Device-talking packages (`scanner`, `firmware`, `setters`) ship in two
layers: a public `…WithOptions` / `New(opts)` entry point that builds a
`*shellyclient.Client`, and a `…OnClient(ctx, client, …)` seam that
accepts a pre-built client. New tests should drive the OnClient seam
against a `httptest.NewServer` so they don't hit the network. Anything
time-sensitive (`LastSeen`, `AuthLockedUntil`, `CheckedAt`) takes a
`clock.Clock`; pass `clock.NewFake(anchor)` and `Advance(d)` for
deterministic assertions. See ARCHITECTURE.md "Testability seams" for
the broader pattern and `helpers_test.go` for the fixture.

### Frontend

```bash
cd web
npm test            # Vitest + jsdom — one-shot
npm run test:watch  # Vitest watch mode
```

Tests live next to the code they cover (`src/**/*.test.ts`). The harness uses
jsdom so DOM-touching helpers run without a real browser.

> Local quirk: vitest 4.x can fail to start its workers if the project
> path contains a literal `+` — the worker module URL ends up
> double-decoding `%20+%20` as a space. CI runs in a clean path and is
> unaffected. If you hit this locally, move the working tree to a path
> without `+`.

### Bundle-size budget

After `npm run build`, CI enforces a raw + gzip budget on the generated JS
and CSS. Run it locally before pushing a change that grows the bundle:

```bash
cd web
npm run build
npm run check:bundle-size
```

The budgets live in `web/scripts/check-bundle-size.mjs` and should only be
raised intentionally, with a note in the PR explaining the growth.

## Development Mode

Development should allow:

- frontend served from disk or dev server
- backend run locally

Production should use:

- embedded frontend assets in the Go binary

## Design Principles

- single-container deployment remains the primary product target
- SQLite remains the default and expected datastore
- workflows should optimize for operational safety over automation
- provisioning should stay guided-first

## Current Gaps

The architecture is documented, but some product-facing pieces are still in progress:

- export flows (devices, logs, templates, jobs)
- advanced mode gating in settings
- richer preview/dry-run flows for risky operations
- broader action discovery for device components where protocol support is reliable
