# Development

## Tooling

Backend:

- Go 1.24+

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
```

Services that orchestrate background jobs (`internal/services/app_jobs.go`,
`internal/services/app_backup.go`) have representative happy-path and failure
coverage. Add new tests alongside the existing files when you extend those
flows.

### Frontend

```bash
cd web
npm test            # Vitest + jsdom — one-shot
npm run test:watch  # Vitest watch mode
```

Tests live next to the code they cover (`src/**/*.test.ts`). The harness uses
jsdom so DOM-touching helpers run without a real browser.

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
