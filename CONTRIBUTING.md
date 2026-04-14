# Contributing

Thanks for helping with ShellyAdmin.

## Project Scope

ShellyAdmin is a trusted-LAN device administration tool. Changes should preserve the current operating assumptions:

- single-container deployment
- SQLite as the default datastore
- one trusted operator
- risky actions kept explicit and reviewable

If a change pushes beyond that scope, please explain the tradeoff clearly in the issue or pull request.

## Local Development

Requirements:

- Go 1.24+
- Node 20+

Backend:

```bash
make dev-backend
```

Frontend:

```bash
cd web
npm ci
npm run dev
```

The Vite dev server proxies `/api` and `/health` to `127.0.0.1:8080`.

`make dev-backend` is for disposable local development only. It uses development-only credentials and should not be treated as a production deployment pattern.

## Build And Test

Production-style build:

```bash
make build
```

Backend tests:

```bash
go test ./...
```

Frontend production build:

```bash
cd web
npm ci
npm run build
```

## Docker

The public container flow is documented in [docs/DEPLOYMENT.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/docs/DEPLOYMENT.md).

Please keep both paths working when you change packaging:

- `docker run` against `ghcr.io/buliwyf42/shellyadmin`
- `docker compose -f docker/docker-compose.yml up -d`

## Pull Requests

Please keep pull requests focused and include:

- the user-visible change
- any security or migration implications
- test coverage added or manual verification performed
- screenshots for meaningful UI changes

## Release Notes

Public releases are tracked in [CHANGELOG.md](/Users/buliwyf/Documents/Codex%20+%20Code%20Projects/shellyadmin/CHANGELOG.md).

When a change should appear in release notes, mention it in the pull request description.
