# Development

## Tooling

Backend:

- Go 1.23+

Frontend:

- Node 20+

## Local Workflow

### Backend

```bash
SHELLYADMIN_PASS=dev-secret go run ./cmd/shellyctl
```

### Frontend

```bash
cd web
npm install
npm run dev
```

## Production Build

```bash
make build
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

- guided provisioning UX
- advanced mode gating
- export flows
- Docker network guidance in the UI
- richer preview flows for risky operations
