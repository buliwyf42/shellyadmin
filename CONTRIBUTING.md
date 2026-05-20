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

- Go 1.25+ (the floor moved from 1.24 → 1.25 in v0.1.16; gin's HTTP/3 transitive deps require it)
- Node 20+
- `golangci-lint` v2 (the project's `.golangci.yml` is v2 syntax — v1 binaries fail to load it). Install: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.0`

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
go vet ./...
golangci-lint run ./...
```

Frontend tests:

```bash
cd web
npm ci
npm test
```

Frontend production build and bundle-size budget:

```bash
cd web
npm run build
npm run check:bundle-size
```

CI (`.github/workflows/test.yml`) runs all of the above — Go tests, Vitest,
the Vite build, and the bundle-size gate — on every push and PR to `main`.
Keep them passing locally before opening a PR.

## Docker

The public container flow is documented in [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

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

Public releases are tracked in [CHANGELOG.md](CHANGELOG.md).

When a change should appear in release notes, mention it in the pull request description.

### CHANGELOG header convention

Each release entry starts with a header in one of two forms:

```
## [0.1.8] - 2026-05-07
## [0.1.8] - 2026-05-07 — Quick summary of the release
```

The `.github/workflows/publish-image.yml` workflow auto-creates a GitHub Release on every `v*` tag push and pulls the entry body (lines between this header and the next `##`) as the release notes. If the header carries an em-dash subtitle (second form), the release title becomes `vX.Y.Z — <subtitle>`. Without a subtitle, the title is just `vX.Y.Z`.

Release tags matching `v*-rc*`, `v*-beta*`, or `v*-alpha*` are automatically marked as prereleases.
