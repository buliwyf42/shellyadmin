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
