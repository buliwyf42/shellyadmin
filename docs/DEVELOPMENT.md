# Development

## Tooling

Backend:

- Go 1.25+ (the `go.mod` floor; moved from 1.24 → 1.25 in v0.1.16
  because gin v1.12.0 pulls `quic-go/quic-go` for HTTP/3). CI and the
  Docker build use the **Go 1.26 toolchain** as of v0.3.4.
- `golangci-lint` v2.12+ (the v1 → v2 migration landed in v0.1.6; older
  binaries fail to load `.golangci.yml` because the schema changed, and
  v2.6 panics on the Go 1.26 stdlib — `file requires newer Go version
  go1.26`). Install: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`.

Frontend:

- Node 22+ (CI and the Docker build use Node 26)

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

## Deployment Workflow

All edits are made **locally on macOS**. Production runs the image as a
**compose stack** named `shellyadmin`, managed through a container-management
UI on the Docker host. The stack files live on the host under
`<stacks-dir>/shellyadmin/{compose.yaml,.env}`.

### Release path

1. Bump `VERSION` + `web/package.json` + lockfile, update
   `CHANGELOG.md`, commit, lightweight-tag `vX.Y.Z`, push
   `git push origin main vX.Y.Z`.
2. `publish-image.yml` builds and pushes `ghcr.io/buliwyf42/shellyadmin:vX.Y.Z`
   and `:latest` (~17–22 min) and auto-creates the GitHub Release.
3. In the container manager: pull the new image, then "deploy" / "recreate"
   the `shellyadmin` stack. The stack pins `:latest`, so a pull + recreate
   is the full upgrade. Via the MCP server this is
   `pull_image(image="ghcr.io/buliwyf42/shellyadmin:latest")` followed
   by `start_stack(name="shellyadmin")` (or restart_stack if already
   up). SQLite persists across recreates because of the
   host-data-dir → `/data` bind mount.

### Stack shape

- Image: `ghcr.io/buliwyf42/shellyadmin:latest` (pinned by tag mover,
  not by digest — operator pulls explicitly before recreate).
- Ports: `8100:8080` (SPA + HTTP API), `8101:8081` (MCP listener;
  only binds when enabled in Settings UI or env-overridden).
- Volumes: host data dir → `/data` bind mount.
- Networks: an external Docker network (`<external-network>`).
- Hardening: `read_only: true` + `tmpfs: /tmp`, `cap_drop: [ALL]`
  with `cap_add: [CHOWN, DAC_OVERRIDE, SETGID, SETUID, KILL]`,
  `no-new-privileges:true`, `pids_limit: 256`, `init: true`.
  **`KILL` is mandatory when `init: true`** — tini (PID 1, running as
  root) needs `CAP_KILL` to forward SIGTERM across the UID gap to
  `shellyctl` (running as the `shelly` user after `su-exec` in the
  entrypoint). Without it, `[FATAL tini (1)] Unexpected error when
  forwarding signal: 'Operation not permitted'` fires on every
  graceful stop, the deferred `runtime_locks.Release` never runs,
  and the next boot panic-loops on a stranded lock row until the
  staleness window (60 s on v0.3.3+, 5 min on v0.3.0–0.3.2)
  expires. Discovered the hard way on the v0.3.3 production deploy
  (2026-05-12).
- Env in `.env` (managed via the container manager's UI, never committed):
  `SHELLYADMIN_PASS_HASH` (argon2id PHC from `shellyctl hash-password`),
  `SHELLYADMIN_SECRET` (cookie secret), `COOKIE_SECURE=false`
  (trusted-LAN posture). `SHELLYADMIN_USER` is **not** set in
  the stack `.env` — it defaults to `admin` from
  [main.go](../cmd/shellyctl/main.go) (`getenv("SHELLYADMIN_USER",
  "admin")`). v0.2.0 removed the **plaintext-password** env var
  (`SHELLYADMIN_PASS`), not the username concept. The login
  handler still compares against `cfg.User` —
  override `SHELLYADMIN_USER` if `admin` is too predictable.

### Pre-deploy snapshot (rollback point)

Before recreating the stack on a release, copy the SQLite file:
`cp <data-dir>/shellyctl.db <data-dir>/shellyctl.db.pre-vX.Y.Z-$(date +%s)`.
These pile up by design as rollback points.

### Historical (pre-v0.2.8)

Before 2026-05-11 production ran the image as a standalone container
via `docker run` driven by `rsync + ssh docker build` from the mac.
That recipe still works for a one-off dev rebuild, but production has
moved to the compose stack. Don't reintroduce the standalone path
as the default deploy in this doc — it would conflict on ports
`8100/8101` with the stack-managed container.

### Versioning at build time

The Dockerfile reads the `VERSION` file at the repo root as the
default version when no `--build-arg VERSION=` is passed. Local
builds show the real version in the navbar and About page. GHCR
builds receive `--build-arg VERSION=` from the git tag.
**On each release, update both `VERSION` and `web/package.json`.**

## Release Cadence Convention

VERSION + `web/package.json` + lockfile bump together on every release. Tag is lightweight (`git tag vX.Y.Z`, no `-a`); push needs `git push origin main vX.Y.Z` because `--follow-tags` only auto-pushes annotated tags. CHANGELOG header convention is `## [X.Y.Z] - YYYY-MM-DD — em-dash subtitle`; the publish-image workflow extracts the subtitle for the auto-created GitHub Release title.

## CI Gates & Branch Protection (v0.3.4)

The repo is on a GitHub Pro personal account — Pro is what makes branch protection + native auto-merge enforceable on a private repo (on Free they can be created but are not enforced); on public repos these features are free. The `main` branch is protected:

- **6 required status checks** (the `name:` fields of the jobs in `.github/workflows/test.yml`): `Release-file version sync`, `Go tests`, `Go vulnerability check`, `Go lint`, `Frontend build`, `Docker image build`. The last one (`image-build` job) smoke-builds `docker/Dockerfile` single-platform amd64 on every PR + push — it closes the gap where a breaking base-image bump previously only surfaced 17-22 min into `publish-image.yml` at release time.
- **Require a PR before merging**, 0 required approvals (0 so solo Dependabot auto-merge isn't blocked on a human review). `enforce_admins=false` — the operator can still push the release commit directly to `main`; the rule binds PRs, not admin. Direct pushes print a warning but go through.
- **Dependabot** (`.github/dependabot.yml`) is grouped (npm dev/prod, gomod, docker, github-actions) and `.github/workflows/dependabot-auto-merge.yml` enables GitHub auto-merge on patch/minor Dependabot PRs — the merge only fires after the required checks pass, so CI is never bypassed. Major bumps stay manual. To regroup pre-existing ungrouped PRs after a config change, comment `@dependabot recreate`.

**Toolchain alignment (v0.3.4):** CI's `setup-go` is `1.26` and `setup-node` is `26`, matching the Dockerfile. `golangci-lint` must be built with a Go ≥ the toolchain it analyzes — v2.6 (built with 1.25) panics on the 1.26 stdlib (`file requires newer Go version go1.26`), so the lint job pins **v2.12**. The `go.mod` directive stays `go 1.25.0`.
