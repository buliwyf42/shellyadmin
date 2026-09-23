# ShellyAdmin — Developer Context

This file is a persistent memory aid for AI-assisted development. Keep it up to date when making architectural decisions.

For deployment workflow, release cadence, and CI/branch-protection
details, see [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

---

## Architecture

- **Backend**: Go 1.25 (single binary, `cmd/shellyctl/main.go`). The Go floor moved from 1.24 → 1.25 in v0.1.16 — gin v1.12.0 pulls `quic-go/quic-go` for HTTP/3, which requires Go 1.25.0 in its `go.mod`. CI's `setup-go` / `setup-node` and the `docker/Dockerfile` base-image tags are held in lockstep by the required **`Toolchain sync`** job in `.github/workflows/test.yml` — read the current toolchain numbers there rather than from prose, which has gone stale twice. Dependabot bumps the image but never the workflow, so before that job existed the two drifted silently (five days in 2026-08/09, PRs #105 → #110). The `go.mod` directive stays `go 1.25.0` — the floor — and the newer toolchain builds it backward-compatibly; the same job fails if a dep bump ever raises that directive past the toolchain CI runs, which is the v0.1.14 breakage above. **The binary links quic-go but does NOT open any UDP/QUIC listener** — the HTTP server in main.go is plain `net/http` over TCP. Phase 2 (v0.2.11) verified this; the QUIC code paths in the binary are dead weight at runtime, not an attack surface.
- **Frontend**: Svelte + TypeScript SPA (`web/src/`)
- **Database**: SQLite via `modernc.org/sqlite` (no CGO required)
- **Deployment**: Multi-stage Docker image — Node builds the frontend, Go builds the backend, Alpine is the runtime. The pinned tags and digests live in `docker/Dockerfile`; the `golang:`/`node:` ones are held to CI by the `Toolchain sync` check, `alpine:` has no CI counterpart and is only ever set there.
- **Entry point**: `cmd/shellyctl/main.go` → `internal/services/app.go`

The SPA is embedded into the Go binary at build time via `//go:embed`.

### Dependency-bump trap (lesson from v0.1.14 / v0.1.15)

A bump of gin or any `golang.org/x/{net,text,sync}` to its latest version can silently raise the `go` directive in `go.mod` past whatever CI is running. v0.1.14 hit this: dep bumps pushed `go.mod` to `1.25.0` while CI was still on Go 1.24, breaking the Test + Publish-Image workflows. Two-line check before any dep bump:

```bash
go list -m -f "{{if .GoVersion}}{{.GoVersion}} {{.Path}}@{{.Version}}{{end}}" all 2>/dev/null | sort -V -r | head -10
grep -E "^go " go.mod
```

If anything in the top of that list outranks `go.mod`'s directive, either bump CI/Dockerfile/go.mod together (the v0.1.16 path) or pick an older version of the offending dep.

### Dependabot groups are all-or-nothing (2026-08-01)

`typescript` is excluded from the `npm-dev-dependencies` group in `.github/dependabot.yml`. Reason: a grouped PR merges as one unit, so a single incompatible member blocks every healthy update beside it. PR #89 bumped `typescript` to `^7.0.2` while `@typescript-eslint/eslint-plugin@8.65.0` still peer-requires `typescript >=4.8.4 <6.1.0` → `npm ERESOLVE`, and it took 10 unrelated dev bumps down with it. Because Dependabot rebuilds the group PR weekly, that was not a one-off but a recurring red PR.

Only the bare `typescript` name is excluded (no wildcard), so `typescript-eslint` and `@typescript-eslint/*` stay in the group — they are not the problem, they are the constraint. `web/package.json` sits on `typescript: ^6.0.3`, and 6.0.3 is the last 6.x release, so the caret can never drift past the peer bound on its own.

Re-add `typescript` to the group once a `typescript-eslint` release accepts TS 7 (check with `npm view @typescript-eslint/eslint-plugin@latest peerDependencies` — 8.68.0 still peers `typescript >=4.8.4 <6.1.0`).

**Resolved 2026-08-28:** `exclude-patterns` does split the dependency out — the 2026-08-03 run opened PR #95 (`typescript 6.0.3 → 7.0.2`) on its own, exactly as Dependabot documents. The 2026-08-02 "unverified" note is settled; TypeScript does not need bumping by hand. But that solo PR is unmergeable *and* rebuilt weekly, so it was the only recurring red run on `main`. An `ignore` rule (`typescript` `>=7`) in `.github/dependabot.yml` now stops it from coming back. Drop the ignore rule together with the group exclusion when TS 7 becomes installable.

### Docker digest bumps look like major bumps (2026-09-02)

`dependabot/fetch-metadata` has no version to compare on a digest-only bump — the tag is unchanged, only the `@sha256` pin moves — so it reports `update-type: version-update:semver-major` with `previous-version` = `` `<sha>` `` and `new-version` = the tag. The patch+minor gate in `.github/workflows/dependabot-auto-merge.yml` skipped those, and they sat green-but-open (PR #107 for a day). The gate now also accepts `package-ecosystem == 'docker' && contains(previous-version, '`')` — the backtick is fetch-metadata's own formatting for a digest, and a real major image bump (`26-alpine` → `27-alpine`) carries a plain version in both fields, so it still goes to manual review. If that formatting ever changes, digest bumps stop auto-merging (fails safe); the fallback is diffing the Dockerfile, which costs a checkout of the untrusted PR ref under `pull_request_target`.

### A grouped PR inherits the HIGHEST update-type — one major blocks the whole group (2026-09-11)

Same gate, third way it misleads. `fetch-metadata` reports **one** `update-type` for a grouped PR, the
highest among its members, so a single major member makes the entire group read
`version-update:semver-major` and the patch+minor gate skips it. PR #117 bundled six dev deps of which
only `vitest`/`@vitest/coverage-v8` went 4.1.11 → 5.0.0; it sat green-but-open for four days.

🩸 **The `previous-version` / `new-version` pair does not belong to the member that set the update-type.**
#117's auto-merge run printed `update-type: version-update:semver-major` next to `previous-version: 8.68.0`
/ `new-version: 8.69.0` — that is `@typescript-eslint/*`, a *minor*. Reading those two lines together says
"the gate is broken"; it is not. Only the PR body's table names the actual major. Check the table, not the
outputs, before touching the gate.

This is working as intended — a major dev-dep bump *should* get eyes — so the fix is operational, not a
code change: a green Dependabot PR that stays open is the gate, not a CI failure. Verify before merging by
running the frontend gates on the PR branch (`npm ci && npm run lint && npm run test:coverage && npm run build`
in a throwaway worktree); for #117 that was 88 tests green and coverage 36.38 % statements, unchanged from
the 4.x baseline, so vitest 5's documented breaking changes (Node ≥ 22 / Vite ≥ 6.4 floors, `sequential`
removed, mocks cleared per default, coverage glob matching, reporters moved to `.vitest/`) touched nothing
here. Note the `Frontend build` job already runs `npm run test:coverage`, so a green check on such a PR is
real evidence — the gate blocked on the *label*, never on an untested change.

### `continue-on-error` steps report `conclusion: success` — the API cannot tell you they fired (2026-09-11)

`.github/workflows/test.yml` has two informational steps (`govulncheck`, the all-deps `npm audit`). For
both, `gh api repos/<o>/<r>/actions/jobs/<id> --jq '.steps[]'` shows `success` **even when the step exited
non-zero** — GitHub rewrites the conclusion, so step conclusions are worthless for checking whether an
informational step found anything. Ground truth is the job log (`##[error]Process completed with exit
code 1` plus the tool's own output), and `gh run view --log` refuses while any job in the run is still
in progress. Verified on PR #120: API said `success`, log showed the audit reporting GHSA-rgw5-rvv9-x895
and exiting 1 while the job stayed green — which is the intended behaviour, but is only visible in the log.

### `gh pr merge --delete-branch` writes to your checkout — that reflog entry is yours (2026-09-12)

`gh` (2.100.0) does not stop at the API call: after merging it checks the base branch out and runs
`git pull --ff-only`. So a session that only ever types `fetch` + `merge --ff-only` still finds

```
09:36:59  checkout: moving from docs/session-lessons to main
09:37:00  pull --ff-only origin main: Fast-forward      <- gh, not a human
09:19:00  merge origin/main: Fast-forward               <- what this session's own updates look like
```

in its reflog, and `Fast-forward / CLAUDE.md | 44 +++` in the terminal — which reads like GitHub
confirming the merge but is `gh` reporting a **write to the local worktree**.

🩸 On 2026-09-12 that entry was taken as proof of a foreign actor in the worktree, on the reasoning
"I don't use `pull`". True of everything typed by hand, and still wrong: **the list of commands you
typed is not the list of commands that ran under your identity.** `gh`, hooks and IDE integrations
all write as you. An unrecognised reflog entry is your own tooling first and a stranger second.

**Concurrency, same hour, the part that nearly cost something.** A second Claude session was
cleaning up the same worktree and deleted eleven stale local branches at 09:40:06; this session's
own `git branch -d` ran 40 s later into "branch not found". Harmless — but its safety filter was
"tip is contained in `main`", and two minutes earlier that filter would have waved through
`docs/session-lessons`, whose commit (09:33:19, the whole content of PR #121) was not in `main` yet
and not pushed. **A "merged into main" filter is blind to exactly one branch: the one someone is
working on right now.** Before deleting branches in a worktree you do not own, check for a foreign
`index.lock` / recent reflog activity, or just announce it first.

### A digest-pinned base image freezes package CVEs, and the pin may already be the newest tag (2026-09-12)

The `v0.6.4` publish run was blocked by the Trivy gate in `publish-image.yml`
(`severity: HIGH,CRITICAL`, `ignore-unfixed: true`, `exit-code: 1`) over
CVE-2026-14456 — `libcrypto3` / `libssl3` 3.5.7-r0, fixed in 3.5.8-r0.

🩸 **The reflex fix does not work here: bump the base-image digest.** The pinned
`alpine:3.24@sha256:28bd5fe8…` **was** the current digest of the `3.24` tag on
Docker Hub — Alpine does not rebuild a point-release image for every package
CVE, so there was no newer base image to move to. Check that before reaching for
a bump, it costs one registry call:

```bash
T=$(curl -s "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/alpine:pull" | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
curl -sI -H "Authorization: Bearer $T" -H "Accept: application/vnd.oci.image.index.v1+json" \
  https://registry-1.docker.io/v2/library/alpine/manifests/3.24 | grep -i docker-content-digest
```

The fix is `apk upgrade --no-cache` ahead of the `apk add` in the runtime stage
(v0.6.5). It gives up nothing that `apk add --no-cache` had not already given
up — package versions were always resolved against the repo, only the base layer
is pinned. Verify the repo actually carries the fix first, rather than hoping:
fetch `https://dl-cdn.alpinelinux.org/alpine/v3.24/main/x86_64/APKINDEX.tar.gz`
and read the `P:`/`V:` pairs. A `.trivyignore` was the alternative and was
rejected — the "not exploitable here" argument (static Go binary, never links
OpenSSL, no QUIC server in the image) is sound but is an argument, and the gate
checks the package set.

🩸 **A failed publish run can still have published something.** `publish-image`
is one job: build+push, sign, scan, extract notes, create Release. The Trivy
failure came *after* push and sign, so `v0.6.4` ended up in GHCR — tagged,
`latest` moved onto it, cosign signature and Rekor entry present — while the
release-notes and Release steps were skipped. The run's conclusion says
`failure` and tells you nothing about which half landed. **Read the step list
(`gh api repos/<o>/<r>/actions/jobs/<id> --jq '.steps[]'`), not the run
conclusion.**

Recovering that state by hand-creating the Release would publish exactly what
the project's own gate rejected. v0.6.5 was cut instead with the fix as its only
content, which also moved `latest` off the unscanned image; the orphaned v0.6.4
image and its `.sig` were deleted from GHCR afterwards. Note that the delete
needs `delete:packages` on the `gh` token (the usual `repo, workflow, read:org,
gist` set is not enough) — otherwise it is a browser job.

### A run-failure list is an archive, not a state (2026-09-14)

`gh run list --status failure` returns every run that ever failed — forever, including one whose
**very next commit on the same PR** fixed it. The GitHub notification inbox shows the same archive.
Two "unread failed runs" reported from a sweep of this repo were both settled history:

- **`ci/go-1.27` is a branch name, not a check name.** It is PR #110: commit `24d4c352` failed at
  12:35 Z on 2026-09-02 with `panic: file requires newer Go version go1.27 (application built with
  go1.26)` (golangci-lint v2.12.2 — the stdlib-lag trap documented in `docs/DEVELOPMENT.md`), commit
  `d4e51788` bumped golangci-lint to v2.13.2 and went green at 12:39 Z, and the PR **merged at
  12:43 Z**. The branch is gone. Eight minutes of red, preserved indefinitely.
- **`v0.6.4`** is the Trivy-gate failure described two sections above, superseded by v0.6.5.

Read the state, not the archive: `gh run list --branch <b>` (newest run wins) plus
`gh pr list --state open`. Three traps came with this one:

🩸 **The count was the window edge, not a finding.** "Exactly two failures" came from `--limit 10`;
`--limit 20` returns 20, back to 2026-07-22. A limit reads like a result.

🩸 **A branch name reads like a check name.** The required checks are in
`gh api repos/<o>/<r>/branches/main/protection` — currently `Release-file version sync`, `Go tests`,
`Go vulnerability check`, `Go lint`, `Frontend build`, `Docker image build`, `Toolchain sync`. None is
named after a branch, so the hypothesis "that red check is silently blocking Dependabot auto-merge"
was structurally impossible before it was worth measuring (and there were zero open PRs anyway).

🩸 **Read this file before reaching for `gh`.** Both causes were already written down here and in
`docs/DEVELOPMENT.md`; the sweep that raised the alarm had not read either, and handed a settled
state on as an open item. The repo memory is the first query, the API the second.

### Deploying a release to the running container — measured, 2026-09-16 (v1.2.0)

A merge is not a deploy. The instance runs a published image, so `main` moving changes nothing
about it; v1.2.0's fix sat on `main` for a day while the container kept serving v1.1.1 and the
defect with it.

**For a pure image update, use the container manager's one-step update action** (Dockhand
`batch_update_containers`), not `down_stack` + `start_stack`. The down/up pair is what
`docs/DEVELOPMENT.md` used to prescribe and what went wrong on this container on 2026-07-22 — it
stayed down for minutes and did not come back reliably. The one-step update pulls, recreates and
starts in one go: measured 2026-09-16, the new container was `running` + `healthy` within ~6 s,
`RestartCount 0`, ports, caps, bind mount and Traefik labels preserved. `start_stack` on a running
stack remains a no-op that pulls nothing.

🩸 **Verify against the container's `ImageID`, never the tool's return value and never the
container's own labels.** `batch_update_containers` returns `{"success": true}` for a no-op as
readily as for a real swap; it also returns a **new container id**, so keep reading the one it
names. And the recreated container's `Config.Labels` still advertised
`org.opencontainers.image.version: v0.6.0` — stale by two releases, as it had been while v1.1.1 ran.
The chain that actually holds: container `Image` → `list_images` entry with that id → its
`repoDigests` → the manifest digest GHCR serves for the tag. On 2026-09-16 that was
`97c0e766…` → `@sha256:bfe15089…` = the digest of both `v1.2.0` and `latest`, image label
`version: v1.2.0`, `revision: 596b7e95…` (the release commit).

🩸 **The image is not the point — the answer is.** A green build proves the build. Finish with a
read-only call that exercises the new code: for v1.2.0 that was `scan_status` returning `job_id` /
`started_at`, absent before the deploy and present after. No scan needs to be started for this.

🩸 **An MCP client's session dies with the container it was talking to, and the error looks like an
outage.** After the recreate, every `scan_status` through the already-connected MCP client returned
`Session terminated` while the server was demonstrably fine (container `healthy`, log line
`MCP server starting addr 0.0.0.0:8081`). That is a client-side Streamable-HTTP session pointing at
a process that no longer exists; it needs a client reconnect, not a fix on the server. A fresh HTTP
session against the same listener answered immediately. **Check the service first — `RestartCount`,
health, the startup log — before concluding the deploy broke something.**

**Release path itself** is `docs/DEVELOPMENT.md`; the one addition from this cut: with the auto-mode
guard active, a direct release push to `main` is refused as a CI bypass. Routing the release commit
through a PR (squash, the repo's usual shape) runs the seven required checks over it and lands the
same content — strictly stricter than the admin bypass the docs allow, and the better default.

### MCP server (HTTP + stdio, opt-in)

Lives in `internal/mcp/`. Two transports share the same 21-tool surface:

- **HTTP** (default for remote access): `cmd/shellyctl/main.go` starts a second listener on `:8081` (configurable via `SHELLYADMIN_MCP_PORT` / `SHELLYADMIN_MCP_BIND`) that speaks Streamable HTTP MCP. Token comes from one of two sources, resolved in this order: (1) `SHELLYADMIN_MCP_TOKEN` env var (operator override, always wins; via `services.DecodeSecretValue` so `_FILE` indirection works), (2) `AppSettings.MCPEnabled && AppSettings.MCPToken != ""` from the persisted settings (added in v0.1.20). When neither is set, the listener does not bind. Authenticated by the resolved token via either `Authorization: Bearer <token>` header **or** a URL whose first path segment IS the token (e.g. `http://host:8081/<token>/` — same shape Home Assistant uses, ergonomic for `mcp-remote`-style clients). Both checks run through `subtle.ConstantTimeCompare`; the matched path prefix is stripped before reaching the SDK handler.
- **Stdio** (v0.2.3+, for Claude Desktop on the same host): `shellyctl mcp` subcommand. `cmd/shellyctl/mcp_stdio.go` opens the database, builds a minimal AppService (no background workers — query session, not server), and serves over `mcp.StdioTransport` via `internal/mcp.RunStdio`. No transport-level auth — the parent process spawning the binary IS the trust boundary; host filesystem permissions on the data dir are the remaining gate. Logs to stderr; stdout carries JSON-RPC frames. SQLite WAL mode handles concurrent readers if a long-running HTTP-mode container shares the same data dir.

- **Surface (v0.2.3)**: 21 tools. **13 read-only**: list_devices, get_device, list_device_actions, scan_status, firmware_status, firmware_install_status, list_templates, get_template, list_credentials, get_settings, get_logs, export_device, compliance_summary. **8 state-changing, all confirm-gated**: refresh_device, refresh_all_devices, start_scan, confirm_scan, firmware_check, firmware_install, execute_device_action, bulk_action. All thin adapters over `services.AppService`. Hard exclusion: anything that mutates ShellyAdmin's *own* config (save_settings, save_credential, save_template, provision, clear_logs).
- **`list_devices` field projection (v1.0.1).** The DeviceListView declares 59 keys per device, so
  an unfiltered fleet listing rendered ~50 KB at 44 devices — the client refused it and spilled to a
  temp file, and consumers went back to `jq` over a dump instead of calling the tool. A `limit` input
  existed but the payload was oversized before it could help. Fixed with a `fields:` allowlist
  (`projectViews` in `internal/mcp/tools.go`) rather than `offset`, because most callers want three
  or four columns, not the first N rows: measured 49,765 B → 4,709 B for the same 44 devices at
  four requested fields. `mac` is always kept — a row without its key cannot be acted on — and an
  unknown field name is an error listing the valid ones, so a typo surfaces at the call instead of
  looking like missing data.
- **`firmware_status` paging (v0.2.3)**: optional `status` / `has_update` / `search` / `limit` / `offset` inputs; output adds `filtered_total` (post-filter) and `returned` (post-page) alongside the unchanged `running` / `done` / `total` job-level metrics. Matters past ~200 devices where the unfiltered payload approaches MCP per-tool output caps.
- **Confirm-flow contract** (added v0.1.22, see `internal/mcp/tools_actions.go` `confirmPolicy`): every state-changing tool has a `Confirm bool` input. Without `confirm: true` the tool returns a typed preview (`SimpleActionResult.Preview=true` + per-tool fields like target counts, risk levels, per-target eligibility from `PreviewBulkAction`) and does NOT call the underlying AppService method. With `confirm: true` it executes. Each call audit-logs `mode=preview` or `mode=confirmed` so operators can pair them by request_id. `actionTool` wraps the context with `services.WithRisk(ctx, "low|medium|high")` so audit rows carry `risk_level`. The tool description includes a verbatim "OPERATOR APPROVAL REQUIRED" policy paragraph telling the LLM to summarize and ask before passing confirm=true.
- **Secret hygiene**: `list_credentials` and `get_settings` route through `internal/mcp/redact.go`. Plaintext password and HA1 hashes never leave the process via MCP. New fields with secret material must add a redactor before they're exposed.
- **Audit**: every tool call logs through `service.LogCtx(ctx, ...)`; `X-Request-ID` is honored on the request and echoed back. Audit rows show in `/api/logs` with `mcp ` prefix, filterable by request_id.
- **Why a separate port** (not `/mcp` on `:8080`): the MCP auth path stays off the cookie + CSRF middleware chain that protects the SPA, and an MCP listener bind failure is isolated from the main UI.
- **`scan_status` carries `job_id` / `started_at` (v1.2.0)** so a caller can tell its own sweep from one the SPA started. Scans have two triggers and no scheduler, and `running: false` is equally true of your finished scan and of a foreign one whose row has not flipped — see the measurement-protocol note further down. Pass-through from the job row, no new state. A new tool that reports on a job should carry the same two fields rather than leaving the caller to guess whose job it is.
- **`scan_status` returns slim pending entries** (`{mac, ip, name, model, gen, app}` only, not full `models.Device`) — full payload was ~63 KB on a 44-device fleet and tripped MCP client output caps. The SPA shape is unchanged. If you add another tool that returns lists keyed off `models.Device`, follow the same pattern (`internal/mcp/tools.go` `slimScanPending` / `ScanPendingItem`).
- **Target resolution** for `get_device` / `list_device_actions` / `export_device` accepts MAC, IP, **or device name** (`services.GetDeviceDetail`). Don't reintroduce a MAC/IP-only check there — the tool descriptions advertise all three.
- **MCP token in settings is encrypted at rest** via `internal/core/secretbox`. `services.SaveSettings` seals; `services.GetSettings` opens. The API GET handler in `internal/api/handler.go` re-redacts to `services.MCPTokenRedacted` (`"<set>"`) before sending to the SPA. When the SPA round-trips settings unchanged, sending `"<set>"` back means "keep the existing token" (the magic value to preserve, not a literal token). Don't expose plaintext `MCPToken` over any new API surface — add a redactor first.
- **MCP listener lifecycle is live** as of v0.1.21. `services.AppService` owns an `*MCPController` (`internal/services/app_mcp.go`) holding the `*http.Server`. `SaveSettings` calls `ReconcileMCPFromSettings` after persisting, which serializes start/stop/rotate transitions on a controller-local mutex. Env-locked instances ignore reconcile. `cmd/shellyctl/main.go` injects `mcp.Build` as the builder via `SetMCPParams` to avoid a services↔mcp import cycle. `Stop(ctx)` tears the listener down before draining background workers. **One AppService is shared between main.go and the API handlers** via `api.Config.Service` — don't reintroduce a second `services.NewAppService(...)` inside the handler, the controller's live state would split.
- **`api.Config.Service`** is the way the api package consumes the shared AppService. When set, NewHandler reuses it; when nil, NewHandler still falls back to constructing its own (kept for tests that don't need to share state). main.go always sets it.

See [docs/adr/0011-mcp-read-only-server.md](./docs/adr/0011-mcp-read-only-server.md) for the full design rationale, the v0.1.22 state-changing tools addendum, and the v0.2.3 stdio + paging follow-up.

---

## Shelly Device Generations

Only Gen2+ devices are supported. Gen1 devices (HTTP REST / GET-based API) are not supported and will not be probed or provisioned.

| Gen   | Protocol                           | Endpoint |
| ----- | ---------------------------------- | -------- |
| Gen2+ | JSON-RPC 2.0 (POST with JSON body) | `/rpc`   |

Generation is detected via `GET /shelly` → `{"gen": N}`. Defaults to Gen2 if absent or zero.

---

## Shelly API Quirks

### Method-not-found error code

Shelly uses **non-standard JSON-RPC error code `404`** (not `-32601`) when a method is not supported on a specific device model. Example response:

```json
{ "error": { "code": 404, "message": "Not Found" } }
```

`isMethodNotFound()` in `provisioner.go` handles both `404` and `-32601` for safety.

### Firmware 2.0.0 auth — brute-force protection → `429`

Firmware **2.0.0** (2026-07-13) added device-side brute-force protection plus RFC-7616-compliant nonce management. Fleet operations (`bulk_action`, `refresh_all_devices`) run against a device with **wrong or stale credentials** can trip a per-device lockout, surfaced as HTTP `429 Too Many Requests`. No code fix is needed — `shellyclient` is already 2.0.0-shaped: RFC-7616 Digest with per-client nonce reuse + `nc` counter, a **stale-nonce retry** (re-parses the fresh challenge on a second 401, `client.go:316-358`), and `429` pass-through (`client.go:355`). Operational takeaway only: repeated wrong-credential attempts across the fleet will lock devices, so fix the credential before re-running a bulk job.

### OTA failures: `premature end of data` — cause UNKNOWN, do not guess again

Fleet OTAs to 2.0.0 fail at random progress percentages with a device-side error:

```
hos_http_client.cpp: Finished; bytes 326/45056, code 200, status -1: Connection error: -14
shos_ota.cpp:650     Update failed (DATA_LOSS: ZIP flush error : premature end of data), will not reboot
```

The device's download from `fwcdn.shelly.cloud` truncates. **The root cause is not known.**
What is established (2026-07-17):

- **Not the CDN.** The same URL fetched from the LAN returns the full image, 3× in a row:
  3,814,926 bytes, `Content-Length` matches, valid zip.
- **Not ShellyAdmin's polling.** Same device (`.129`, RSSI −58), same firmware: failed at 12%
  while polled every 5s, failed at 37% with no polling at all — identical error both times.
- **Signal is a factor, not the whole story.** `.59` sits at **−78 dBm** (the device's own
  roam threshold is −80) and fails early plus times out on ordinary RPCs. But `.129` at
  −58 dBm fails too.
- **It is not universal.** `.92` (−50 dBm) completed 0→100% in 2:25 once, and four devices
  self-updated overnight via Shelly's phased cloud rollout.
- **Untested lead:** curl on a LAN host could not verify `fwcdn.shelly.cloud`'s certificate
  ("unable to get local issuer certificate"), which would fit TLS interception somewhere in
  the path. The devices pin `shelly_cloud.pem`. Unverified.

**A prior version of this section claimed the cause was ShellyAdmin polling the device
during the download. That was wrong** — it rested on comparing two *different* devices
(`.92` unpolled vs `.59` polled) and attributing the difference to polling, when `.59` also
has the worst signal in the fleet and fails unpolled. v0.5.6 shipped that claim in its
CHANGELOG; v0.5.7 retracted it. If you are tempted to explain these failures from one
suggestive pair of runs, don't — get the A/B on one device first.

**Gotcha for any future measurement:** the Docker host is multi-homed and the production
ShellyAdmin reaches the IoT VLAN as **`192.168.211.88`**, polling every device roughly every
60s. An "unpolled" experiment against the live fleet is not unpolled unless that instance is
stopped. Check the source IPs in the device log before trusting a result.

**Diagnostic:** `curl -N http://<ip>/debug/log` streams the device's log as plain text over
HTTP — no config change, no MQTT/websocket detour. It carries `ota_begin` / `ota_progress` /
`ota_success` / `ota_error` events and the resolved `fwcdn.shelly.cloud` URL, and is by far
the fastest way to see what an OTA is actually doing.

`FirmwareInstallQuietPeriod` (default 150s) keeps the job off the device between trigger and
reboot. Keep it — the version cannot change during the download, so polling then buys
nothing — but it is hygiene, **not** a fix for the failures above.

### Update availability is a version comparison, not a string compare

`firmware.IsNewer` (x/mod/semver, `internal/core/firmware/firmware.go`) decides
`Result.StableUpdate` / `BetaUpdate`. It must never go back to `!=`: a device on a beta sits
**ahead** of its model's stable channel, so string inequality advertises the older stable as
an available update. During the phased 2.0.0 rollout that mislabelled **36 of 44** fleet
devices (on `2.0.0-beta3`, offered stable `1.7.5` / `1.7.99-powerstripg4prod1` /
`1.8.99-plugmg3prod0`), and each triggered install was silently ignored by the device.

Shelly versions are semver-shaped, so prerelease < release gives `2.0.0-beta3 < 2.0.0`
correctly. Unparseable versions fall back to string inequality — an odd vendor string can
fail to suppress a downgrade, but can never *hide* a real update.

**The rollout is phased per device, not per model** — do not diagnose it as a device fault.
Two identical `S4PL-00416EU` strips, checked the same minute (2026-07-21): `shelly-strip4-01`
was offered stable `2.0.0`, `shelly-strip4-02` still stable `1.7.99-powerstripg4prod1`
(build `20250819`, the factory image). Same for their `alt` `PowerStripZB` entry. Ground
truth is the device's own `Shelly.CheckForUpdate` — the server answers per device id, so a
missing update on one unit says nothing about the model. Nothing to fix; wait for the bucket.

### The firmware index: `updates.shelly.cloud` — and why it can't shortcut a phased rollout

Contrary to the forum consensus that Gen2+ has no offline update path, there **is** a public index:

```
curl -k https://updates.shelly.cloud/update/<APP>     # APP = Shelly.GetDeviceInfo → "app", e.g. Mini1PMG3
→ {"stable":{"version","build_id","url"},"beta":{…},"alt":…,"time":…}
```

`-k` is required — `updates.` and `fwcdn.shelly.cloud` serve certificates from Allterco's internal CA, so
curl fails with a bare `https://` (exit 60, and a *silent* `000` if you don't check the status). The `url`
points at `https://fwcdn.shelly.cloud/gen2-ntest/<APP>/<sha256>` and serves the ZIP to a LAN host fine
(verified 2026-07-22: `200`, `Content-Type: application/zip`, 3,435,534 B for Mini1PMG3 beta3). That image
can be handed to a device via `Shelly.Update{"url": …}` — the one param ShellyAdmin does **not** send
(`TriggerUpdateOnClient` passes only `stage`).

**But the index does not carry the build the fleet is actually running.** Checked 2026-07-22 across all 12
fleet apps while 27 of 44 devices ran stable `2.0.0` (`20260710-101127/2.0.0-g87fbfa4`): **every** app
still advertised stable `1.7.5` (or the model's factory build) and beta `2.0.0-beta3`. 2.0.0 stable
appeared for **no app at all**. Device-identifying query params (`id`, `uid`, `mac`, `device_id`, `ver`)
do not change the answer. So a `url` install cannot pull a device forward; every reachable URL is either a
downgrade or the build it already runs. **Do not re-derive this.**

**RESOLVED 2026-09-05: it was a pause, and the fleet has converged.** The index snapshot the earlier
note asked for was taken 54 days later and answers it — every Gen3/Gen4 fleet app now serves stable
`2.0.0` (build `20260710-…`, i.e. the *same* build the manual OTA installed) plus beta **`2.0.1-beta1`**
(`20260819-…`; the beta slot moved on again to `2.0.1-beta2`, `20260910-125922`, checked 2026-09-11 — stable
unchanged at `2.0.0`). `2.0.0-beta3` is gone from the index entirely: the beta slot moved on. So the stable
channel did serve 1.7.5 on 2026-07-22 and serves 2.0.0 today — a withdrawal-then-resume of the phased
rollout, never announced either way. The census matches: **44 devices polled directly (`/shelly`, no auth
needed for `ver`) → 40× `2.0.0`, 4× `1.7.5` (the frozen Plus line), 0× `2.0.0-beta3`.** All 44 now carry
`fw_auto_update: stable` (two did on 2026-07-22), so the resumed stable channel carried them; no manual
install was needed and none is documented.

**The 2.0.1 betas need no code work here (checked 2026-09-11, beta2).** Both `2.0.1-beta1` and `-beta2` are
pure bugfix releases per Shelly's Gen2 changelog: no new RPC methods, no new `Shelly.GetStatus` /
`CheckForUpdate` / `Schedule` fields, no breaking changes — so nothing at the `sysAltVariants()` /
`firmware.*` seams moves. The one entry that looks like it touches us, *"Authentication: Echo digest
`algorithm` only when the challenge carried it"*, is device-side: `shellyclient` parses `algorithm` out of
the challenge and defaults to `MD5` when absent (`client.go:420`, `:441`), which stays RFC-7616-conform
either way. Re-read the changelog per release rather than assuming this holds — 2.0.0 itself added
`sys.alt` and `sys.provisioning`.

**2.0.1 went stable on 2026-09-23 and holds the same verdict.** The final changelog carries only *Fixed*
and *Local web → Fixed* — no Added / Changed / Breaking section, no new RPC method or config/status field.
The auth entry above shipped as-is, plus a device-side nonce-table slot leak on TTL expiry; the rest is
HTTP hardening (chunked-encoding overflow, use-after-free under flood), ADE7953 / Pro3EM / ProEM metering,
BLE/Matter and per-model fixes. Index the same day: `Mini1PMG3` and `PlugSG3` serve stable `2.0.1`
(`20260923-…/2.0.1-ge1a198b`), `Plus1` stays at `1.7.5`. The operator triggered the fleet update the same
day; the version census, not the click, is what says it landed.

🩸 **Uptime stopped being able to date the install, and that is the transferable part.** The 2026-07-22
reasoning leaned on "a firmware change reboots, so uptime dates the install" — sound only while reboots
are otherwise rare. Since then a documented reboot campaign (2026-09-03, eight devices rebooted to unstick
them from the wrong AP) overwrote exactly the clocks that carried the evidence: 24 of 44 devices now show
an uptime younger than 2026-07-22, far more than the 13 that were on beta3, and nothing distinguishes an
update-reboot from a maintenance-reboot. **A monotonic clock that any unrelated action resets is a dating
method with a shelf life** — read it early or not at all. The version census needs no dating and settled it.

🩸 **Positive control is mandatory on this endpoint** (same lesson as 2026-09-04): a freely invented app
name answers `HTTP 404 (Unknown firmware application)` — byte-identical to a real app with a missing
variant. Query a known-good app in the same run, or a dead endpoint reads as a finding.

Where the index *is* worth using: it's the missing piece for the `premature end of data` failures above —
fetch the ZIP once to a LAN host, serve it (`python3 -m http.server`), point `Shelly.Update{url}` at plain
`http://`, and the device never touches `fwcdn.shelly.cloud`. That sidesteps the whole untested TLS-
interception lead. Not yet tried against a real failure — there has been nothing to install since.

Second, unrelated use: **the index dates EOL hardware without trusting a vendor blog post.** `Plus1` and
`Plus2PM` are the only fleet apps with **no `beta` key at all**, while every Gen3/Gen4 app carries
`2.0.0-beta3` — independent confirmation that the Gen2 Plus line is frozen at 1.7.5.

🩸 **That signal has a shelf life too: a missing `beta` key is not a frozen line.** On 2026-09-23, right
after 2.0.1 went stable, `Mini1PMG3` and `PlugSG3` answered with **no `beta` key either** — the beta slot is
simply empty between cycles. Read on that day, the check above would have declared current Gen3 hardware
EOL. `firmware.IsFeatureFrozen` is a static SKU allowlist and never depended on it; keep it that way, and
treat an absent beta only as corroboration taken while the Gen3/Gen4 apps *do* carry one.

### The stored IP goes stale silently — `online: true` outlives reachability (2026-09-05)

The Device row's `ip` is written by a **scan**, and nothing re-checks it between scans. When a DHCP lease
wanders, the row keeps the old address and every later job talks to the wrong host. Measured on
`shelly-strip4-02`: the firmware job logged `no route to host` for `192.168.211.117` while `/api/devices`
still reported `online: true`, `last_refresh_error: ""` and a plausible `fw: 2.0.0` — all of it carried
over from the scan of 2026-09-02. The device was fine the whole time at **`192.168.211.187`**, and Home
Assistant never noticed because its Shelly entries are zeroconf-sourced and follow the name.

🩸 **So a failing job and a healthy inventory row are not in contradiction here — they are reading
different clocks.** `online`/`last_seen`/`fw` are scan-time snapshots; only `fw_checked_at` and a job's own
error text are live. Ground truth for "where is this device now" is mDNS, not the DB:

```bash
dscacheutil -q host -a name shelly-<name>.local     # macOS; getent hosts on Linux
curl -s http://<ip>/shelly | jq -c '{name,mac,ver}' # no auth needed for the version
```

A rescan repairs the row.

**PARTLY FIXED (2026-09-12): the row no longer lies, the IP still is not re-resolved.** Reading the
code showed the harm was mostly not the stale IP but how a failed check was written:
`runFirmwareJob` persisted `result.StableVer` / `BetaVer` / `CheckedAt` **unconditionally**, so a
check against an unreachable device blanked the firmware cache *and* stamped a fresh `fw_checked_at`
— the row looked freshly verified precisely when nothing had been reached — while reachability was
never touched at all. That is the whole `online: true` + empty `last_refresh_error` symptom above,
and it also fed `device_surface.go`, whose bulk actions gate on `Online` and therefore kept firing at
the dead address. `applyCheckResult` (`internal/services/jobs/firmware_check.go`) now writes the
cache only on success and records what was actually learned on failure, with `firmware.Result.Unreachable`
separating silence from a refusal — an auth challenge or lockout proves the device is alive and must
not count as a miss. Reachability semantics mirror the refresh path: second consecutive silence takes
it offline.

🩸 **The mDNS re-resolve was deliberately NOT built, and the reason is worth keeping.** Two things
have to hold before it is worth a line of code, and neither was measured: (1) this deployment runs
`enable_mdns: false` — the whole mDNS path is switched off, so a re-resolve built on it would be dead
code here; (2) `scanner.ScanMDNS` browses by raw multicast (`browseMDNS`, own `dnsmessage` packets)
but then resolves the discovered `.local` name with **`net.DefaultResolver`** — the system resolver —
and the runtime image is `alpine` + musl with no `nss-mdns`, where `.local` goes to the configured
unicast DNS and may simply NXDOMAIN. If the mDNS scan path is ever wanted in the container, measure
that resolve first (`getent hosts shelly-x.local` inside the running container); if it fails, the fix
is to read the A record out of the mDNS answer that `browseMDNS` already receives, not to add an mDNS
dependency.

### 🩸 Three ways the rescan repairs nothing (or breaks something else) — all look like success (2026-09-05)

Repairing the stale row above turned out to be booby-trapped three times over. All three sit in the
*normal* path, and none of them announces itself.

**(1) `scan_status` serves the PREVIOUS scan's pending list, and it is indistinguishable from a
fresh one.** No timestamp, no "stale" marker, `running: false` either way. Calling it before
starting a scan returned 43 devices with `shelly-strip4-02` at its **old** `.117` — confirming that
list would have written the very defect back into the row that the rescan was supposed to fix. The
repair tool would have re-introduced the bug and reported success. **Always `start_scan` first and
re-read `scan_status` afterwards; compare a known-changed field before confirming.**

**(2) `ConfirmScan(macs)` is not "register these" — `UpsertDevices` treats the argument as the
COMPLETE scan result.** Every existing device *not* in the slice gets `LastRefreshOK=false`,
`LastRefreshError="refresh timed out"`, `ConsecutiveMisses++`, and at ≥2 misses `Online=false`
(`internal/db/devices.go:118-132`). So the intuitive move — confirm only the one MAC you came to
fix — silently marks the other 43 as failing. **The narrow list is the dangerous one here**, the
inversion of the usual "never run a mass operation without an explicit ID list" rule. Corollary:
never confirm a scan that found fewer devices than the inventory holds, and count found-vs-total
before confirming, not after.

**(3) ANY fleet-wide `UpsertDevices` silently discards the firmware-check cache — `ConfirmScan` is just the usual way to trigger one.**
`UpsertDevices` writes the scan-probed `models.Device` wholesale, and a scan probe does not carry
`FWAutoUpdate` / `FWCheckedAt` / `FWAvailableStable` / `FWAvailableBeta`. Measured on the successful
2026-09-05 run: `fw_auto_update` went from **44× `stable` to 43× empty**, the only survivor being the
one device the confirm did *not* register. Empty means "never read" (see ADR-0009), so the inventory
afterwards claims auto-update was never configured on the whole fleet — while the devices themselves
still hold their `Shelly.Update{stage:"stable"}` schedules, unchanged. It is a cache, and
`firmware_check` repopulates it, but nothing tells the operator to run one.

**FIXED (2026-09-12).** `UpsertDevices` now carries the four `FW*` fields over from the existing
row, alongside `DeviceNum` / `FirstSeen` — the recommended option, not the "run a firmware check
afterwards" workaround. **`TLSAllowInsecure` was blanked by the same line and is carried too**: it is
operator-set and no scan probe reports it, so every confirmed scan silently reset the TLS opt-out.
The specification was the sibling path — `jobs/refresh.go` had been carrying exactly these five since
it was written, and only the scan path was missing them; a defect found by comparing two callers of
the same write, not by reading the failing one. Guarded by
`TestUpsertDevicesPreservesFirmwareCacheAndTLSOptOut`, which fails on all five without the fix.

**Corollary that only showed up because two sessions overlapped:** the FW cache is **not durable
state**, it is whatever the last writer left. On 2026-09-05 a *second* session (audit `request_id`
`b5556e06a73ef975`) ran `start_scan` at 11:12:06 and `confirm_scan` at 11:16:07, five minutes after
this session's `firmware_check` (`cd5dfe64d61428a7`, 11:13:02) had repopulated 43 of 44 rows — and
wiped all 44 again. Two lessons: **read `get_logs` filtered on `mcp action` before concluding that
an unexplained write was yours** (the `request_id` separates actors cleanly, and a `confirm_scan`
appearing with no preview in front of it is a different client); and **do not treat
`fw_auto_update` as evidence of configuration** — for that, read `Schedule.List` on the device and
look for `Shelly.Update` with `origin: "shelly_service"`, which is the durable truth.

Both traps bit on the same run: the 2026-09-05 scan found **42 of 44** (`shelly-strip4-02` was in a
scheduled power-off window, `shelly-hz2` was missed outright, see below), so it was correctly left
**unconfirmed** — a confirm would have penalised two healthy rows and still not fixed the IP.

### `shelly-hz2` is missed by the subnet scan — a tail-latency event against the 2 s `scan_timeout` (2026-09-13)

Three consecutive `192.168.211.0/24` scans (2026-09-02, 09-05, 09-12) came back without
`FC:E8:C0:DB:19:50` / `192.168.211.47`, while its twin `hz1` (`.102`, same `SPEM-003CEBEU63`,
same subnet) is found every time. Until 2026-09-12 the suspicion pointed at the probe path in
`internal/core/scanner/scanner.go`. **That is wrong — the scanner is exonerated.**

The measurement that settled it: the *same* `ScanSubnets` code with the *same* production
parameters (concurrency 64, timeout 2 s), run from a laptop on the same LAN, found **43 devices
including `.47` in 8 s**. The production container, minutes earlier, found **41 without `.47`
and took over 45 s** for the identical 254 addresses.

Exonerated, in order, each by its own measurement — **do not re-walk these**:

| Suspect | How it was cleared |
| --- | --- |
| Address enumeration | `ExpandCIDR("192.168.211.0/24")` yields 254 addresses and contains `.47` |
| The "not a Shelly" skip (`mac == "" && gen == 0`) | `/shelly` returns a complete payload: `mac FCE8C0DB1950`, `gen 2`, in 32 ms |
| Auth | `auth_en: false`; `Shelly.GetDeviceInfo` / `GetConfig` / `GetStatus` all `200` unauthenticated |
| Reachability from the server | a targeted `refresh_device` from the *same container* succeeded minutes after the sweep missed it (`last_refresh_ok: true`), as did that day's `firmware_check` |
| The sweep code itself | identical code + parameters find the device reliably from another host |
| "it is the Ethernet-only devices" | `hz2` is wired with `wifi.status: disconnected`, but so are `hz1` and `pro-3em-workshop` (`.70`), both found every sweep |

What is left is the **latency tail of the device itself**, not the network position — see the
2026-09-13 measurement below. The 8 s vs 45 s for the same address range is a real difference
between the two vantage points, but it is not what drops `.47`.

🩸 The transferable part: **"the code is the same" is not "the run is the same".** A sweep is a
measurement of the network between the scanner and the target, and the scanning host is part of the
apparatus. Reproducing from a second vantage point separated the two in one run, after two sessions
had been looking in the wrong file.

**The A/B that was run the same evening — and why its conclusion does not hold:**

| Run | `scan_timeout` | Container | Result |
| --- | --- | --- | --- |
| 13:35 | 2 s | v0.6.0, 8 days uptime | 41 found, `.47` **missing** |
| ~20:10 | 5 s | v1.1.1, minutes old | 44 found, `.47` **present** |
| ~20:15 | 2 s (restored) | v1.1.1, minutes old | 44 found, `.47` **present** |

🩸 **The trap in that sequence is still worth more than its finding.** Changing `scan_timeout` 2 → 5
produced exactly the expected result, and concluding "the timeout was it" would have been wrong on
the evidence available: a redeploy had happened between the 13:35 run and the evening ones, so the
two measurements differed in **two** variables, not one. **When an experiment confirms your
hypothesis on the first try, check what else moved since the baseline — and run the A/B/A, not the
A/B.**

🩸 **But the A/B/A's own conclusion — "at 2 s the fresh container finds it too, so the timeout is
not the knob" — does not survive either, and the reason is the same class of error one level up:
every cell in that table has n = 1, and the fault is intermittent at roughly 2 of 3.** Three single
draws from a coin that lands "found" about a third of the time are consistent with pure chance in
both directions. **A single run per cell cannot support a conclusion about an intermittent fault —
not a positive one, and not a negative one.** The "container uptime" successor hypothesis rested
entirely on those two evening runs.

### 2026-09-13: it is a tail-latency event, measured with a control

Six sweeps in 15 minutes, **nothing changed** (`scan_timeout` 2 s, `scan_concurrency` 64, container
v1.1.1 at ~15 h uptime, no `confirm_scan`):

| Sweep | Found | `.47` |
| --- | --- | --- |
| 1 | 44 | present |
| 2 | 43 | **missing** |
| 3 | 43 | **missing** |
| 4 | 44 | present |
| 5 | 43 | **missing** |
| 6 | 42 | **missing** (plus `.218`) |

That settles two things at once. **The uptime hypothesis is dead** — the miss is fully present at
15 h, minutes after a sweep that found the device — and **the miss rate is high enough (4/6) that
any future A/B needs n ≈ 6 per cell**, which is the first time this question has had a usable
baseline.

The mechanism, each step measured:

| Step | Measurement |
| --- | --- |
| `.47` carries the highest baseline load in the fleet | 60 s capture of `/debug/log` on both twins: **12 RPC/min on `.47` vs 4 on `.102`** — `.47` is evcc's Verbrauchsmessung, `.102` is not |
| A Pro 3EM serialises HTTP and degrades under concurrency | 6 parallel `/shelly` GETs, no sweep running: **30 ms single-shot → 1.0–1.1 s max** on all three Pro 3EMs |
| The sweep probe collides with the 60 s refresh | device log during a sweep: 6 refresh RPCs and 2 sweep RPCs from `192.168.211.88` inside **one second** |
| The tail crosses the timeout — and only on `.47` | `/shelly` timed from a third host throughout one sweep: `.47` **max 2.078 s**, `.102` **max 1.064 s**; medians identical at **31 ms**, p90 54 vs 50 ms |

`scan_timeout` is **2 s**. `.47`'s tail reaches **2.078 s**; its identical twin under the identical
sweep tops out at 1.064 s and is never missed. The distributions differ **only** in the tail, which
is exactly where the timeout sits.

🩸 **This is why every measurement since 2026-09-05 found nothing.** The "`.47` answers in 30 ms"
figure that anchored the whole investigation is the **median**, and it was never in conflict with
the miss — the median and the p90 are healthy on the device that drops out. **A timeout is a
statement about the tail; a median, however many times you repeat it, cannot refute one.** The
44/44 `curl` sweep of 2026-09-12 measured the same median from the same wrong angle.

🩸 **The probe changed the finding, and that was the confirmation.** Sweep 6 ran while two latency
loops were hitting the twins, and it lost a **second** device (`.218`). More load → more misses is
a dose-response, so read the absolute tail figures as *including* this session's own ~12 req/s, not
as pristine values.

**Measured and rejected** — the three candidates the previous revision listed as unmeasured, all on
VM 114 during sweeps: neighbour table (`table_fulls 0`, `forced_gc_runs 0`, `unresolved_discards 0`,
105 entries against `gc_thresh1 128`), conntrack (**1413 of 262144**), fd/sockets (175 sockets
against a 1024 soft limit). None is saturated. The exhaustion story was wrong.

**What was NOT yet proven at that point**: that raising the budget removes the miss. The decisive
experiment was `scan_timeout` **2 → 5 s**, then **≥ 6 sweeps** against the 4/6 baseline. It has now
been run — see below. `scan_concurrency` 64 → 32 is the alternative knob on the same mechanism.
**Both need the Settings page** (`save_settings` is deliberately absent from the MCP), and that needs
an interactive login — there is no 1Password item for `shellyadmin.home.lan`, so that step belongs to
the operator.

### 2026-09-13, same day: the 5 s budget was set and measured — the miss is halved, not gone

The operator set `scan_timeout` **2 → 5 s** via the Settings page. Read back before measuring:
`scan_timeout 5`, `scan_concurrency` unchanged at **64**, same container (`StartedAt
2026-09-12T18:04:10Z`, `RestartCount 0`) — **one variable moved**, which is what the 2026-09-12 A/B
failed to guarantee. Seven sweeps, 14:10–14:24, no `confirm_scan`, no active probing of the twins:

| Sweep | Found | `.47` | Sweep finished within |
| --- | --- | --- | --- |
| 1 | 44 | present | ≤ 90 s (not timed) |
| 2 | 44 | present | — |
| 3 | 43 | **missing** | — |
| 4 | 44 | present | ≤ 32 s |
| 5 | 44 | present | ≤ 27 s |
| 6 | 43 | **missing** | ≤ 27 s |
| 7 | 44 | present | ≤ 27 s |

**2 misses in 7, against 4 in 6 at 2 s.** The rate roughly halved, and the device is still missed.

🩸 **Say what that does and does not establish.** Against the baseline read as a *fixed* rate of
2/3 — which the 09-02 / 09-05 / 09-12 history also supports — P(≤ 2 misses in 7) = 99/2187 ≈ **4.5 %**,
so the improvement is real at the very edge of significance. Against the baseline read as what it
actually is, **a six-run sample**, Fisher's exact on 5/7 vs 2/6 hits gives **p ≈ 0.21** — nothing at
all. **Both readings are honest, and the weaker one is the fairer one**; more sweeps at 5 s cannot
fix that, because the power is capped by the six-run baseline, not by the new arm. The one outcome
that would have settled it on this design was **zero** misses in six ((1/3)^6 ≈ 0.14 %), and it did
not occur. **It did occur on 2026-09-14 — at 10 s / 32, with both knobs moved at once, so it settles
the comparison against 2 s and nothing about 5 s vs 10 s. See that section before citing this one.**

**The mechanism is now measured directly on the wire, not inferred.** `tcpdump` on VM 114
(`ens19`, passive, no extra load on the devices) during sweep 7, SYN **and** SYN-ACK:

```
0.00 -> SYN     :51094      <- the sweep probe
1.05 -> SYN     :51094      retransmit, unanswered
2.08 -> SYN     :51094      retransmit, unanswered   (a 2 s budget dies here)
3.10 -> SYN     :51094
3.10 <- SYN-ACK :80         handshake completes after 3.10 s
```

`.47` answered the sweep's **TCP handshake alone** after **3.10 s** — above the old 2 s budget,
inside the new 5 s one, and that sweep found the device. **This is the single cleanest confirmation
the whole investigation has produced**: the excursion is where the timeout sits, and the 2.078 s
figure from the HTTP-level timing was an *underestimate* of it, not the ceiling. In the two sweeps
that missed, `.47` had connection attempts retransmitting unanswered for **6.3 s and 11.2 s**
(`.102`, same sweep, same capture: answered on the first SYN). So the remaining misses are the part
of the tail that reaches past 5 s.

**The feared price did not materialise — and the model behind it was wrong.** Every timed sweep
finished within ~30 s, against "over 45 s" measured at 2 s on 2026-09-12. Reason, from the same
captures: of 254 addresses only **105 ever emit a TCP SYN at all**; the other 149 have no ARP answer
and fail before any HTTP timeout can apply, and of the 105, the ~61 non-Shelly hosts answer on the
first SYN. **"23 dead addresses × 5 s" was arithmetic over a set that does not reach the timeout.**
Raising the budget further is therefore close to free, which is the relevant input for the next
decision — it is the operator's, not the scanner's.

Until the tail is covered, every `ConfirmScan` still costs hz2 an undeserved miss (see trap 2) — now
with a measured probability of roughly **two in seven** rather than two in three.

🩸 **Method note that outlives this bug: `tcpdump` on the scanning host is the only complete record
of what a sweep did.** `get_logs` is not (see below), `shellyctl.log` carries no `[scan]` lines at
all (`grep -c scan` → 0), and `scan_status` reports the *verdict* without the evidence. One passive
capture answered in a single sweep what eight days of application-level timing could not.

🩸 **Tool trap found while measuring this: `get_logs` is not a complete record of a sweep.** It
returned no `[scan]` line at all for `.47` from the six sweeps run on 2026-09-13 — neither the
successes nor the misses — while continuing to serve older `[scan]` lines for the same device from
the periodic refresh minutes earlier. Those lines are in **neither** `/docker/shellyadmin/shellyctl.log`
(`grep -c '211\.47'` → **0**) **nor** `docker logs shellyadmin`, so `get_logs` serves some other
buffer, and it drops entries under sweep load. Mechanism not established — but the operational rule
is: **the absence of a `[scan]` line in `get_logs` is not evidence that an address was not probed.**
Ground truth for what a sweep saw is the `pending` list of `scan_status`, nothing else. Two earlier
sessions could have been misled by this in the opposite direction.

### 2026-09-14: zero misses in six at 10 s / 32 — but two knobs moved, so the cause is not attributable

Read back from `get_settings` before measuring: `scan_timeout` **10** (was 5), `scan_concurrency`
**32** (was 64). Both were changed by the operator between 09-13 and 09-14, and nothing records why.
Six sweeps, `start_scan` each time, `scan_status` read only *after* `running: false`, no
`confirm_scan`, no active probing of the twins:

| Sweep | Found | `.47` |
| --- | --- | --- |
| 1-6 | 44 each | **present in all six** |

**0 misses in 6**, against 2 in 7 at 5 s and 4 in 6 at 2 s.

🩸 **This is the outcome the 09-13 section named as decisive — and it still does not decide what the
previous revision hoped it would.** Against the baseline read as a fixed 2/3 miss rate, `(1/3)^6 ≈
0.14 %`, and against the 2 s arm directly Fisher's exact gives **p ≈ 0.061 two-tailed** (0.030
one-tailed): the run is clearly better than 2 s. Against the **5 s** arm it is worth nothing —
Fisher on 0/6 vs 2/7 gives **p ≈ 0.27**, and `(5/7)^6 ≈ 13 %` of six-sweep runs at the 5 s rate
would show zero misses by chance. So "10 s fixed what 5 s did not" is exactly the claim this data
cannot carry.

🩸 **And the attribution is gone regardless of the statistics, because `scan_timeout` AND
`scan_concurrency` moved together.** Halving the concurrency reduces the self-inflicted load the
09-13 capture showed colliding with the 60 s refresh; doubling the budget covers more of the tail.
Either alone could produce this table. **The 2026-09-12 A/B was retracted for this exact reason and
the same error is now baked into the 10 s arm** — a one-knob run is the only thing that would fix
it, and nobody has run one. Note this is the *second* time the fix and the measurement were applied
in the same step; if the budget gets raised again, move one knob.

🩸 **New trap, and it invalidates the yardstick every earlier table in this section used: `found: 44`
is no longer "all".** The inventory holds **45** devices (`list_devices` → `total: 45`);
`shelly-strip4-02` (`48:F6:EE:DD:47:8C`, `.187`) is absent from all six `pending` lists while
`/api/devices` reports it `online: true`. It is **not** a scanner miss and **not** the stale-IP bug
— verified with a positive control from a host outside the container: `.187` gives no HTTP answer at
all (`000` after 4 s) and **does not resolve over mDNS**, while `.47` and `.102` answer in ~30 ms and
`shelly-strip4-01` resolves normally. No HTTP *and* no mDNS, with the controls live, is a powered-off
device — its documented scheduled power-off window (see the 09-05 note). So the six sweeps found all
44 *reachable* devices.

**The transferable part: a sweep result is a fraction, and this section spent twelve days writing
down only the numerator.** `44` meant "complete" on 2026-09-02 and means "one short" on 2026-09-14,
without either number changing its appearance. Read `list_devices` → `total` in the same run, and say
"44 of 45, one verifiably powered off" — never a bare count. This also re-arms trap 2 above: a
`ConfirmScan` on any of these six sweeps would have penalised `strip4-02` for being switched off.

### 2026-09-14/15: the one-knob run at 10 s / 64 — and why its headline number must not be quoted

The missing arm. `scan_concurrency` was set back to **64** with `scan_timeout` left at **10**, so this
run differs from the 5 s arm in the timeout alone and from the 10 s / 32 arm in the concurrency alone.
Read back from `get_settings` before the first sweep (`scan_timeout 10`, `scan_concurrency 64`) — and
read back again after an earlier attempt showed **32**, i.e. the operator's first save had not reached
the server. **Read the setting back; do not measure on the assumption that a save landed.**

Twelve sweeps, exclusive access, no `confirm_scan`:

| Sweep | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `.47` | ✓ | ✓ | ✗ | ✓ | ✓ | ✓ | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ |

🩸 **6 of 12 is not a rate, and quoting it as one is the mistake this section exists to prevent.**
The series is **not stationary**: sweeps 1-7 carry **one** miss, sweeps 8-12 carry **five, consecutively**.
A run of five at the series-wide rate of 0.5 has probability ≈ 3 %, and the break sits cleanly between 7
and 8. Two supporting observations: **only `.47` degrades** — never a second device in any of the five,
so a general network or container saturation is ruled out — and `.47` answered **10 of 10** direct probes
at 30-46 ms immediately afterwards, with the `.102` control identical. The device is healthy at rest;
the median still cannot see the tail (same lesson as 09-13).

**The most likely cause is the measurement itself.** Thirteen sweeps of 254 addresses at concurrency 64,
roughly every two minutes for 25 minutes, on top of the 60 s refresh and evcc's 12 RPC/min — and a Pro 3EM
serialises HTTP (measured 09-13: 6 parallel GETs → 1.0-1.1 s). Cumulative load fits the time course.
**Not measured** — that needs a `tcpdump` during the late sweeps, which is the obvious next step.

**Consequence for every comparison in this section: arms of different lengths are not comparable.**
Arm A ran 7 sweeps, the 10 s / 32 arm 6, this one 12. If the miss rate climbs with series length, a
longer arm partly measures its own length. Restricted to the first seven sweeps — comparable in both
count and elapsed time:

| Arm | `scan_timeout` | `scan_concurrency` | Misses |
| --- | --- | --- | --- |
| A (09-13) | 5 s | 64 | 2 in 7 |
| B (09-14) | 10 s | 32 | 0 in 6 |
| C (09-14/15) | 10 s | 64 | 1 in 7 |

Fisher's exact on every pair: **nothing significant** (C vs A and C vs B both p ≈ 1.0). So the one-knob
run that was supposed to isolate the timeout shows **no timeout effect** — and cannot rule one out either.
The honest summary is not "10 s does not help" but **"this experimental design cannot answer the question,
and now we know which variable broke it."** A future run must hold the sweep count, the cadence and the
elapsed time equal across arms, or interleave the arms rather than running them back to back.

🩸 **Method: `scan_status` cannot tell you whose scan it is reporting, and that cost this session a wrong
conclusion.** A first attempt at this arm ran while the operator still had the ShellyAdmin UI open. Scans
have exactly two triggers — `internal/mcp/tools_actions.go` (MCP) and `internal/api/handler_scan_firmware.go`
(the SPA); there is no scheduler. A foreign scan starting between a `scan_status` read and the next
`start_scan` produces `scan already running`, which reads exactly like "your own previous sweep is still
going" — and was misread that way here, discarding a valid sweep and crediting a foreign sweep's miss to
this series. `ScanStatus()` and `StartScan()` both read `GetLatestJob("scan")`, so they cannot disagree
about the *same* job; a disagreement means a *new* job appeared. The protocol that survives this:
**take exclusive access first**, then per sweep `start_scan` → wait → `scan_status` → and use the *next*
`start_scan` as the freshness proof — if it is rejected, discard that sweep rather than counting it.
`running: false` is not a freshness proof (see trap 1 above).

**FIXED in v1.2.0 (2026-09-16):** `scan_status` now returns `job_id` and `started_at` — the job row's
`ID` / `CreatedAt`, passed straight through `jobs.ScanStatus` and `mcp.ScanStatusOutput`. The protocol
above still holds, but it now has a cheap check instead of a discipline: read `job_id` after
`start_scan`, and compare it on every poll. A different id means a foreign sweep, and `job_id: 0` means
no scan job exists at all. Guarded by `TestScanStatusIdentifiesTheJobItDescribes`. **Only true against a
deployed v1.2.0+** — the fix sat on `main` for a day while the container kept running v1.1.1 and the
failure mode with it; a merge is not a deploy.

**On the inventory count:** the 09-14 note above cites 45 devices. Do not carry that number forward — a
hardware swap was in progress during these runs, and the fleet size moves. **The count is a snapshot; the
ground truth is the comparison against the inventory in the same run** (`list_devices` → `total`), together
with a reachability check for anything missing. Two devices were legitimately absent across these series —
one in a scheduled power-off window, one switched off mid-series — and both were confirmed powered off by
the no-HTTP-plus-no-mDNS control, not assumed.

### OTA configuration on Gen2+ — implemented via `Schedule.*`, not `OTA.SetConfig`

The Shelly Gen2 API has **no `OTA.SetConfig` / `Sys.SetAutoUpdate` / dedicated OTA-config method**. The `OTA.*` methods that DO exist (`OTA.Start/Write/Data/Abort/Commit/Revert`) are byte-level chunked-upload plumbing, not configuration. Direct firmware update lives at:

- `Shelly.Update` — one-shot firmware update (requires `stage` param: `"stable"` or `"beta"`)
- `Shelly.CheckForUpdate` — check for available updates (returns BOTH `stable` and `beta` in one response)

**Auto-update is implemented as a Schedule entry.** The device's local web UI ("Enable auto update firmware", added in firmware 1.2.0) does NOT call a dedicated method. Instead it creates a `Schedule.*` job that calls `Shelly.Update` on a recurring timer with `origin: "shelly_service"` as the marker. ShellyAdmin reads/writes auto-update state through this same mechanism (see `internal/core/firmware/autoupdate.go`):

- **Read**: `Schedule.List` → filter for `calls[].origin == "shelly_service"` AND `calls[].method == "Shelly.Update"`. The `params.stage` field tells you `stable` or `beta`. Absent or disabled → `off`.
- **Set stable/beta**: `Schedule.Create` with `enable: true`, `timespec: "0 0 0 * * 0,1,2,3,4,5,6"` (cron-style; daily at midnight), `calls: [{method: "Shelly.Update", params: {stage: <stable|beta>}, origin: "shelly_service"}]`.
- **Disable**: `Schedule.Delete` for the matching job id.

**`stage` is a slot, not a floor.** `stage: "beta"` installs whatever the server lists under
`beta` — it does **not** mean "beta or anything newer". A device parked on `2.0.0-beta3` with
`stage: "beta"` is a permanent no-op (its beta slot already matches) and will *not* pick up
`2.0.0` when that lands, because the final release ships in the **stable** slot. For a device
sitting ahead of its own stable channel during a phased rollout, `stage: "stable"` is the
correct setting: it no-ops while the offered stable is older (the device ignores the install,
see the rollout note above) and installs the new build the moment the rollout reaches it.

Persisted on the Device row as `fw_auto_update` with values `""` (never read) | `off` | `stable` | `beta`. Read during every firmware check job. Bulk-settable via the Firmware page's "Auto → Off / Stable / Beta" buttons (action `set_auto_update`). Surfaced in compliance via the `auto_update_stage` rule.

### Alternative firmware variants — `sys.alt` (firmware 2.0.0+), read-only

Firmware 2.0.0-beta3 added an `alt` object: alternative firmware **variants** for the same hardware — a Zigbee or Matter build, or an add-on profile (e.g. Power Strip Gen4 → `PowerStripZB` "with Zigbee", Mini 1PM Gen4 → `Mini1PMG4ZB`, Pro 3EM → `Pro3EMProAddon`). It is a **map** keyed by variant id → `{name, desc, stable?{version,build_id}, beta?{version,build_id}}`, NOT a scalar third channel.

**Source is `Shelly.GetStatus` → `sys.alt`**, not `Shelly.CheckForUpdate` — a deliberate choice, not the only option: as of firmware **2.0.0 stable** (2026-07-13) the `alt` object *is* also carried in `Shelly.CheckForUpdate` (during the betas it lived only in sys status, so the earlier note that "CheckForUpdate stayed stable+beta only" is now outdated). We keep reading `sys.alt` because it's free — the scanner already fetches `Shelly.GetStatus` into `Device.RawStatus`, so **no extra RPC and no new DB column** — `sysAltVariants()` in `internal/services/actions.go` derives `Device.FWAlt []models.AltFirmwareVariant` from the cached RawStatus at `GetDevices()` time, exactly like `SwitchCount`. Surfaced in `/api/devices` (DeviceListView) + MCP `get_device`/`list_devices`, and as an `alt: <id>` badge in the Model cell of the Firmware page.

**Read-only, by design.** `Shelly.Update` accepts only `stage` (stable|beta) or `url` — there is **no `stage:"alt"`** and the alt object carries **no `url`**. So a variant/protocol switch (e.g. flashing a plug to Zigbee firmware) is NOT wired and can't be, with the currently documented API. ShellyAdmin only *shows* which devices could switch (useful for the ZHA fleet); the actual switch is done via the device's own web UI. If Shelly later documents an install path, wiring lives at the `TriggerUpdate*` seam in `internal/core/firmware/firmware.go`.

`sys.provisioning` (secure-provisioning state, same firmware) rides the same RawStatus read: `sysProvisioning()` → `Device.Provisioning map[string]any`, surfaced in `get_device`. Absent fleet-wide until a device is enrolled in secure provisioning.

### Feature-frozen firmware lines — static allowlist by model SKU

Some Gen2 "Plus" device lines are feature-frozen per Shelly's [Firmware Update Policy](https://shelly-api-docs.shelly.cloud/gen2/General/FirmwareUpdatePolicy/) — they keep working and get critical bug fixes, but will **never** receive 2.0.0+ (no end date given anywhere Shelly publishes). `firmware.IsFeatureFrozen(model)` (`internal/core/firmware/firmware.go`) answers this from a static `map[string]struct{}` keyed by **model SKU**, not the `app` string — Shelly's own docs list the frozen lines only by marketing name, no SKU or `app`-identifier table, so the SKUs were sourced from [aioshelly](https://github.com/home-assistant-libs/aioshelly) (`internal/const.py`, checked 2026-07-22), Home Assistant's actively-maintained Shelly library, cross-checked against the policy page's device names. The `Plus1`/`Plus2PM` entries are additionally confirmed against this fleet's own devices (`fw_available_beta == ""` on both).

Two policy-listed lines are deliberately **not** in the allowlist:
- **Plus i4's DC variant** (`SNSN-0D24X`) — the policy only names "Plus i4", not the DC variant separately; freezing it too is a plausible but unconfirmed inference.
- **BLU Gateway Gen3** (`S3GW-1DBT001`) — a different generation from the Gen2 "BLU Gateway" the policy means.

Extend the list only after confirming a new SKU (via a real device's `Shelly.GetDeviceInfo`, or a corroborated third-party source) — never by guessing; a wrong entry either silently mislabels a still-supported device or misses a genuinely frozen one.

`Device.FWFrozen` mirrors this at `GetDevices()` time (`fwFrozen()` in `internal/services/actions.go`, same derived/no-migration pattern as `FWAlt`/`Provisioning` — must run **before** `compliance.Evaluate` in `app.go`, since the opt-in `flag_frozen_firmware` compliance rule reads it). Surfaced as a `frozen` badge next to the `alt:` badge on the Firmware page, and via `get_device`/`list_devices`/`compliance_summary` with zero extra MCP code. Purely informational per ADR-0002 — never gates the install button, and the compliance rule defaults off.

Historical context: the `ota` provisioner section and an `ota_auto_update` compliance field that called `OTA.SetConfig` were **fully removed in v0.0.16** (the v0.0.14 removal was partial). If an `ota` block still appears in a user-supplied JSON template, it falls through to the catch-all handler (calls `Ota.SetConfig` → 404 → gracefully skipped).

### Model SKU → marketing name lookup (frontend-only)

The Model column on the Firmware and Devices pages only ever had the raw SKU (e.g. `SNSW-001X16EU`) or Shelly's own `app` code (e.g. `Plus1PM`) to show — neither is the human name a user recognizes. `modelName(sku)` in `web/src/lib/shellyModels.ts` resolves a SKU to its marketing name (e.g. `"Shelly Plus 1"`) from the same [aioshelly](https://github.com/home-assistant-libs/aioshelly) `const.py` `MODEL_NAMES` table used for the feature-frozen allowlist above (144 entries, checked 2026-07-22). One aioshelly SKU constant (`MODEL_WALL_DISPLAY_X2I`) ships with a leading-space typo in the upstream source — stripped when building the table here.

Deliberately **frontend-only**: this is pure display formatting, not business logic, so there's no Go lookup, no `Device` field, no migration — `modelName()` is called directly in `Firmware.svelte`, `devices/DeviceTable.svelte`, and `DeviceDetail.svelte` at render time (tooltip text + the model-column fallback display when `app` is empty). Contrast with `IsFeatureFrozen` above, which had to live server-side because it feeds a compliance rule.

### mqtt.ssl_ca valid values

The `mqtt.ssl_ca` field only accepts exactly four values:

- `""` / omitted — no TLS
- `"*"` — TLS, disable certificate validation
- `"ca.pem"` — TLS with built-in CA bundle
- `"user_ca.pem"` — TLS with user-uploaded CA certificate

### WS SSL CA

Same four-value pattern as MQTT: `""`, `"*"`, `"ca.pem"`, `"user_ca.pem"`.

---

## Key Files

| File                                       | Role                                                                          |
| ------------------------------------------ | ----------------------------------------------------------------------------- |
| `internal/services/app.go`                 | Service layer; job scheduling, refresh/scan orchestration                     |
| `internal/services/device_surface.go`      | Bulk actions (set_sntp_server, reboot, etc.)                                  |
| `internal/core/scanner/scanner.go`         | Device discovery & probing; populates `models.Device`                         |
| `internal/core/firmware/firmware.go`       | `Shelly.GetDeviceInfo` + `Shelly.CheckForUpdate` per channel; install trigger |
| `internal/core/firmware/autoupdate.go`     | `Schedule.*`-based auto-update read/write (see ADR-0009)                      |
| `internal/core/firmware/methods.go`        | `Shelly.ListMethods` capability probe (see ADR-0010)                          |
| `internal/core/provisioner/provisioner.go` | Template-based fleet provisioning                                             |
| `internal/core/compliance/compliance.go`   | Compliance rule evaluation                                                    |
| `internal/core/setters/setters.go`         | Targeted single-field setters for bulk actions                                |
| `internal/core/clock/clock.go`             | Tiny `Clock` interface + `Real()` + `Fake.Advance(d)` for deterministic tests |
| `internal/core/secretbox/secretbox.go`     | NaCl secretbox envelope encryption for credential at-rest storage             |
| `internal/middleware/requestid.go`         | `X-Request-ID` middleware; IDs propagate to audit_log rows and slog attrs     |
| `internal/services/password.go`            | Argon2id hash/verify for `SHELLYADMIN_PASS_HASH`                              |
| `internal/services/store.go`               | `Store` interface at the service/DB boundary                                  |
| `internal/api/errors.go`                   | `respondError` / `respondUserError` — sanitized HTTP error responses          |
| `internal/models/device.go`                | Device struct (source of truth for all device fields)                         |
| `internal/models/settings.go`              | ComplianceRules, AppSettings, etc.                                            |
| `internal/mcp/server.go`                   | `Build` — HTTP MCP listener (token-gated, request-id middleware)              |
| `internal/mcp/stdio.go`                    | `RunStdio` — same tool surface over `mcp.StdioTransport` (v0.2.3+)            |
| `internal/mcp/tools.go`                    | Read-only tools + filter/page helpers (firmware_status, list_devices, etc.)   |
| `internal/mcp/tools_actions.go`            | Confirm-gated state-changing tools + `actionTool` audit wrapper (v0.1.22+)    |
| `cmd/shellyctl/mcp_stdio.go`               | `shellyctl mcp` subcommand entry — minimal AppService, stderr-only logs      |
| `web/src/pages/Provision.svelte`           | Provisioning UI — form editor + JSON editor                                   |
| `web/src/pages/provision/`                 | Section forms: Sys, Mqtt, Ws, Ble, Wifi, Eth, Modbus, Zigbee, Scripts, Webhooks (v0.2.4), Cover (v0.2.5), ZigbeeOps (v0.2.6), UserCA |
| `web/src/pages/Compliance.svelte`          | Compliance rules UI                                                           |

---

## Provisioner Template Sections

Sections in a template JSON map to backend handlers in `applySection()`:

| Section key   | Handler                                                                                  |
| ------------- | ---------------------------------------------------------------------------------------- |
| `sys`         | `Sys.SetConfig`                                                                          |
| `mqtt`        | `MQTT.SetConfig`                                                                         |
| `ws`          | `WS.SetConfig`                                                                           |
| `ble`         | `BLE.SetConfig`                                                                          |
| `cloud`       | `Cloud.SetConfig`                                                                        |
| `matter`      | `Matter.SetConfig`                                                                       |
| `wifi`        | `Wifi.SetConfig` (full surface: sta, sta1, roam, static IPv4)                            |
| `auth`        | `Shelly.SetAuth`                                                                         |
| `ota`         | catch-all handler; Shelly returns 404 → `skipped` (form + normalizer removed in v0.0.16) |
| `kvs`         | `KVS.Set` per key                                                                        |
| `script`      | `Script.SetConfig` per id (loop like kvs)                                                |
| `ui`          | `UI.SetConfig`                                                                           |
| `gen2_rpc`    | arbitrary method map                                                                     |
| `gen1_http`   | skipped (legacy; Gen1 no longer supported)                                               |
| anything else | `<Capitalized>.SetConfig`                                                                |

Template variable substitution: `{device_name}` is replaced with the device's configured name (from `Shelly.GetConfig` → `sys.device.name`).

---

## Job Locking

Long-running jobs (refresh, scan, firmware_check) use a SQLite-backed status:

- `"running"` — job active
- `"done"` / `"failed"` — terminal
- `"interrupted"` — set on startup for any jobs stuck in `"running"` from a previous crash

A **stale-job guard** (2-minute timeout) prevents stuck `"running"` jobs from blocking manual triggers. Refresh jobs are **not** auto-restarted on startup (unlike scan/firmware_check) because they are user-initiated.

---

## Compliance Rules

Compliance rules in `models.ComplianceRules` are evaluated in `compliance.Evaluate()`. Key behaviors:

- `cloud_enabled` checks the device's cloud enable setting (distinct from `cloud_connected`)
- Custom rules support `source: device | config | status`, path traversal with `.`, operators: `eq` (default), `ne`, `contains`, `regex`, `exists`
- `{device_name}` token in rule values is substituted with the device's effective name

---

## Testability Pattern: OnClient Seams + Clock

Added in v0.1.15 (M3a). Each device-talking package (`internal/core/{scanner,firmware,setters}`) ships in two layers:

- **Public `…WithOptions` / `New(opts)` entry points** — production callers use these. They build a `*shellyclient.Client` from `Options` and delegate to the seam below. Behavior unchanged from before M3.
- **`…OnClient` seams** — accept a pre-built `*shellyclient.Client` directly. The precedent is `firmware/methods.go:30 ListSupportedMethodsOnClient`; v0.1.15 brought scanner / firmware / setters into line. Tests construct a `httptest.NewServer` fake-Shelly + a `shellyclient.Client` aimed at it, then call the OnClient variant.

`scanner.ProbeOptions` and `firmware.Options` carry an optional `Clock clock.Clock` field — nil falls back to `clock.Real()`. Tests inject `clock.NewFake(t)` + `Advance(d)` to pin timestamp-bearing fields (`LastSeen`, `AuthLockedUntil`, `CheckedAt`) to deterministic values.

When you add a new device-talking call site, follow the same pattern:

1. Public `…WithOptions` builds the client from `Options`.
2. Internal `…OnClient(ctx, client, …)` does the work.
3. Any wall-clock dependency goes through `clk.Now()`, not `time.Now()`.

The shared test fixture for firmware lives at `internal/core/firmware/helpers_test.go` (`fakeShelly` — per-method handler map, call recorder, defaults to the Shelly non-standard 404 RPC error for unregistered methods). Reuse it for new firmware tests.

---

## App Settings (operator-facing)

Defined in `models.AppSettings` (`internal/models/settings.go`), normalised on load via `Normalize()`. Notable knobs:

| Field                         | JSON key                         | Default     | Bounds      | Notes                                                                                         |
| ----------------------------- | -------------------------------- | ----------- | ----------- | --------------------------------------------------------------------------------------------- |
| `FirmwareInstallTimeout`      | `firmware_install_timeout`       | 600 (10 min) | `> 0`, and `≥ quiet + 150` | Per-device cap before install_job marks "unknown". Normalize forces the floor so the timeout can't expire before the first poll |
| `FirmwareInstallQuietPeriod`  | `firmware_install_quiet_period`  | 150         | `(0, 600]` s; 0 = unset → default | How long install_job leaves the device alone after the trigger. Hygiene, **not** a fix — see the OTA-failure section above (v0.5.6 claimed otherwise, v0.5.7 retracted) |
| `FirmwareInstallPollInterval` | `firmware_install_poll_interval` | 5           | `[1, 60]` s | How often the install_job re-queries device firmware **after** the quiet period (added v0.1.13)                                               |
| `FirmwareCheckInterval`       | `firmware_check_interval`        | 0 (off)     | `≥ 0` s     | Periodic firmware_check job cadence; 0 disables the scheduler                                 |

The pattern for adding a new knob:

1. Field on `AppSettings` with JSON tag.
2. Default in `DefaultSettings()`.
3. Bounds clamp in `Normalize()`.
4. A `…FromSettings(s) <Type>` helper in the consuming service (see `firmwareInstallTimeoutFromSettings` / `firmwareInstallPollIntervalFromSettings` in `internal/services/app_jobs.go`).
5. Settings.svelte input — match `firmware_install_timeout`'s number-input shape unless a preset dropdown fits better.
6. TS field on `AppSettings` in `web/src/lib/types.ts`.

---

## Plaintext Password Removed

`SHELLYADMIN_PASS` (plaintext) was removed in **v0.2.0**. v0.0.15 added `_HASH` and started warning on plaintext use; v0.2.0 closed the deprecation window. The argon2id hash/verify in `internal/services/password.go` and the `shellyctl hash-password` subcommand remain — but as of first-run setup (below) `SHELLYADMIN_PASS_HASH` is no longer the *source of truth* for the login, only a one-time import seed.

---

## First-Run Setup — operator login in the DB (ADR-0017)

The operator login (username + argon2id hash) lives in the database, not the environment. See [docs/adr/0017-first-run-setup.md](./docs/adr/0017-first-run-setup.md).

- **Storage**: single-row `admin_credentials` table (migration 031), accessed via `db.{Get,Save,Clear}AdminCredential` and the service helpers in [internal/services/app_auth.go](internal/services/app_auth.go). NOT in `AppSettings` (which is mirrored to the SPA via `GET /api/settings` — a hash must never go there). The PHC hash is one-way, so it is stored verbatim, NOT secretbox-sealed.
- **Resolution**: the login handler resolves the credential at request time via `h.adminCredential()` ([internal/api/handler.go](internal/api/handler.go)) — DB first, then a `cfg.User`/`cfg.PassHash` fallback kept so handler tests that seed `Config` (not the DB) still pass. The lockout/TOTP keys use the *resolved* username.
- **Boot ([cmd/shellyctl/main.go](cmd/shellyctl/main.go))**: the old "panic when `SHELLYADMIN_PASS_HASH` is empty" is GONE. `ImportEnvCredentialOnce(user, passHash)` imports a still-present env hash into the DB exactly once (only when no DB credential exists) — seamless upgrade for existing deployments. With no credential at all the server logs a setup-mode warning and boots anyway.
- **Setup mode**: no credential ⇒ the SPA renders the public setup screen (`/setup`) gated by `GET /api/setup/status` → `{configured: bool}`. `POST /api/setup` is the **only unauthenticated mutation** — public, rate-limited, one-shot (409 once configured), race-guarded by a service mutex + the single-instance lock.
- **Change later**: `POST /api/account/credentials` (authenticated, **cookie-only** — a PAT cannot rotate the login that gates it). Verifies the current password, updates, then revokes all sessions (SPA redirects to `/login`). UI: `web/src/pages/settings/AccountCard.svelte`.
- **Recovery**: `shellyctl reset-auth --force` clears the row → next boot is setup mode again (mirrors `shellyctl unlock --force`). This is the forgotten-password path now that env is no longer authoritative.
- The encryption-key requirement (next section) is unaffected and still enforced — setup mode still needs `SHELLYADMIN_ENCRYPTION_KEY`.

---

## Encryption Key Required (v0.3.0)

S6 from the consolidated review (ADR-0013) closed the encryption-key auto-generation path in v0.3.0. The boot path's `loadEncryptionKey()` in [cmd/shellyctl/main.go](cmd/shellyctl/main.go) refuses to start when neither `SHELLYADMIN_ENCRYPTION_KEY` nor `SHELLYADMIN_ENCRYPTION_KEY_FILE` is set. v0.2.11 added the deprecation warning; v0.3.0 turned it into a hard error.

Migration recipe for operators upgrading from v0.2.x with an auto-generated `{dataDir}/shellyadmin.key`: copy the file contents to a path outside the data volume (Docker secret, NixOS secret store, sops-encrypted file in the homelab config repo), then set `SHELLYADMIN_ENCRYPTION_KEY_FILE=/path/to/it` in the compose `.env`. The startup error message includes the legacy path when it's detected, so the recovery is one `cat` away.

Threat closed: a volume snapshot exfiltrating both the encrypted credentials in `shellyctl.db` AND the key file sitting next to it — defeated the at-rest encryption entirely. External key management = both halves no longer share a backup boundary.

**Key rotation (v0.5.3)**: `shellyctl rotate-key` re-seals every secretbox blob — credentials, credential groups, TOTP material, the MCP token inside the settings JSON — in ONE transaction (`internal/db/rotate.go` `RotateSealedColumns`). Old key from `SHELLYADMIN_ENCRYPTION_KEY[_FILE]`, new key from `SHELLYADMIN_NEW_ENCRYPTION_KEY[_FILE]`; without `--force` it's a dry run that verifies the old key opens everything. Writes a timestamped DB backup before applying; refuses while a fresh runtime-lock heartbeat exists (live server). Explicit-key seal/open variants: `secretbox.{Seal,Open}StringWithKey`. **A new sealed column anywhere must be added to `RotateSealedColumns`**, or rotation silently leaves it orphaned under the old key.

---

## Single-Instance Constraint (ADR-0015, v0.3.0)

The 030 migration adds a `runtime_locks` table; [internal/services/runtimelock](internal/services/runtimelock/runtimelock.go) claims the `primary` row at startup, runs a 60s heartbeat, and releases on graceful shutdown. A second container starting against the same SQLite file finds a fresh row and refuses to boot — the error names the foreign hostname/pid + when the row will go stale.

A stale row (5+ minutes without heartbeat — covers `kill -9`'d previous container) is silently overwritten. Operators who don't want to wait the staleness window can run `shellyctl unlock --force` to clear the row manually.

Why: process-local state (rate-limit map, MCP listener, background workers) doesn't replicate across instances. Two containers reading the same DB would double-spawn the firmware-check scheduler, race audit-log retention, and try to re-bind `:8081`. The lock is the explicit door-closer for that misconfiguration.

---

## TOTP 2FA + Personal Access Tokens (Block 4c, v0.3.0)

Two new operator-facing auth surfaces, both built on the existing server-side session store (S5):

**TOTP 2FA (T1)** — operator enrolls a TOTP secret via the Settings UI; subsequent logins require a 6-digit code in addition to the password. RFC 6238 stdlib impl in [internal/services/totp/totp.go](internal/services/totp/totp.go); 10 single-use backup codes issued at enrollment (sha256-hashed + secretbox-sealed in the DB row); wrong code bumps the same per-account lockout counter as wrong-password. [internal/api/handler_totp.go](internal/api/handler_totp.go) drives the `/api/totp/{status,enroll,verify-enroll,disable}` surface.

**Personal Access Tokens (T3)** — bearer-token credentials for headless callers (Home Assistant, cron, scripts) so `/api/*` mutations don't have to fake the cookie + CSRF dance. Token format `pat_<8hex id>_<64hex random>`. Scope catalog (`admin`, `devices:read/write`, `firmware:read/write`, `provision`, `settings:read/write`) gated per-route via `middleware.RequireScope`. Bearer-authed requests skip CSRF (the token IS the proof-of-intent). [internal/services/tokens/tokens.go](internal/services/tokens/tokens.go) is the orchestration; [internal/middleware/auth.go](internal/middleware/auth.go) extends RequireAuth to honor the `Authorization: Bearer pat_…` header. PAT-authed callers cannot mint, list, or revoke other PATs (privilege-escalation guard at the handler).
