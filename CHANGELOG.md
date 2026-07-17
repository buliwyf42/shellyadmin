# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [0.5.7] - 2026-07-17 — Retract v0.5.6's root cause; patch the build's Go

### Retracted

- **v0.5.6 claimed OTA installs failed because ShellyAdmin polled the device
  during the download. That claim was wrong and is withdrawn.** It rested on
  comparing two *different* devices — one unpolled that succeeded, one polled
  that failed — and crediting the difference to polling. The polled device also
  has the weakest Wi-Fi in the fleet (−78 dBm, against its own −80 roam
  threshold) and fails unpolled too.

  The A/B that should have been run first: one device (RSSI −58), same firmware,
  same trigger — failed at 12% polled every 5s, failed at 37% with no polling,
  both with the identical device-side error `DATA_LOSS: ZIP flush error :
  premature end of data`. Polling is not the cause.

  What is now known about the real failure is recorded in CLAUDE.md: the device's
  download from `fwcdn.shelly.cloud` truncates; the CDN itself serves the full
  3,814,926-byte image fine from the LAN; the root cause is **not identified**.

  The v0.5.6 *code* is unaffected by this retraction — the released image is
  sound, and the changes it shipped stand on their own. Only the stated reason
  for the quiet period was wrong. The knob is kept, redocumented as hygiene:
  the reported version cannot change until the device flashes and reboots, so
  polling during the download learns nothing while costing the device heap.

  v0.5.6's own GitHub Release was never published — its `Trivy image scan` step
  failed (below) after the image had already been pushed and signed.

### Fixed

- **Build's Go stdlib carried CVE-2026-39822** (`os.Root` symlink following,
  HIGH). The `golang:1.26-alpine` base was pinned to a digest shipping Go 1.26.4;
  repinned to Go 1.26.5, which carries the fix. Not exploitable here — the
  codebase never calls `os.Root`/`os.OpenRoot`, and the reachability-based
  `govulncheck` gate was green throughout — but Trivy version-matches the stdlib
  and failed the release, so v0.5.6 published an image without a Release entry.

- **`firmware_install_quiet_period` disagreed with itself.** `Normalize` left a
  stored `0` alone while the job's `…FromSettings` helper read `0` as "unset" and
  substituted 150s, so the persisted value and the effective one diverged — and
  the timeout floor, computed from the stored `0`, under-sized the polling window.
  `Normalize` now substitutes the default too, and the helper's doc no longer
  claims a `0` opt-out it never honoured.

## [0.5.6] - 2026-07-17 — Stop starving the OTA we're waiting for

> **Superseded by v0.5.7.** The root cause asserted below — that polling an
> in-flight OTA starves it — was retracted. The code changes stand; the
> explanation does not. See v0.5.7.

Firmware installs triggered from ShellyAdmin failed on every attempt, while the
same update applied fine from Shelly's cloud rollout or the device's own web UI.
The cause was ShellyAdmin itself, and the fallout from Shelly firmware 2.0.0's
phased rollout made it impossible to see.

### Fixed

- **The install job killed the update it was waiting for.** `installOne` began
  polling `Shelly.GetDeviceInfo` every 5s immediately after triggering
  `Shelly.Update`. A device downloading firmware buffers a ~3.6 MB image with
  almost no heap to spare (~62 KB free on a Gen3), and answering those RPCs
  starves the download — it stalls at 0%, silently, and never recovers. The job
  then waited out its timeout and reported the healthy device as `unknown`
  ("device still on X after 5 min").

  Measured on two identical Plug Gen3s, same firmware, same trigger: unpolled
  reached 100% in **2:25** and rebooted onto 2.0.0; polled every 5s sat at
  **0% for 2.5 min** and never moved. The job now leaves the device strictly
  alone for `firmware_install_quiet_period` before it starts polling — nothing
  can change until the device flashes and reboots anyway.

- **Available-update detection compared version strings, not versions.**
  `StableUpdate`/`BetaUpdate` were `channelVer != currentVer`, so a device
  running a beta — which sits *ahead* of its model's stable channel — was
  offered the older stable as an "update". During the phased 2.0.0 rollout that
  mislabelled 36 of 44 fleet devices (on `2.0.0-beta3`, offered stable `1.7.5` /
  `1.7.99-powerstripg4prod1` / `1.8.99-plugmg3prod0`), and every resulting
  install was silently ignored by the device. Now `firmware.IsNewer` orders
  versions with `x/mod/semver`, which gets prerelease < release right.
  Unparseable versions fall back to string inequality, so an odd vendor string
  can fail to suppress a downgrade but can never hide a real update.

- **A successful install could be reported as `unknown`.** The poll loop required
  an exact match against the version predicted by the last `firmware_check`. A
  device installing anything else polled to the timeout despite having updated.
  Any move off the original version now counts as landed.

### Added

- **`firmware_install_quiet_period`** app setting (default 150s, bounds
  `[0, 600]`): how long the install job leaves a device untouched after the
  trigger. Surfaced in the Settings UI.

### Changed

- **`firmware_install_timeout` default 300 → 600s.** `Normalize` now also forces
  it to at least `quiet_period + 150`, which repairs existing settings rows
  carrying the old 300 — too tight once a quiet period exists.

## [0.5.5] - 2026-07-01 — Dependency roll-up + dead-code cleanup

Housekeeping release: no runtime behaviour change. Bundles the Dependabot
dependency bumps with the ponytail-audit dead-code removal.

### Changed

- **Runtime base image Alpine 3.23 → 3.24** (`docker/Dockerfile`), plus the
  `node:26-alpine` and `golang:1.26-alpine` build-stage digests refreshed.
  Alpine 3.24.1 ships the same CVE-clean OpenSSL (`libcrypto3`/`libssl3`
  3.5.7-r0) that v0.5.4 pinned by digest.
- **Dev-dependency bumps** (`web/`): playwright 1.60→1.61, typescript-eslint
  8.60→8.62, vitest coverage 4.1.8→4.1.9, eslint 10.4→10.6, prettier 3.8→3.9
  (+ eslint-plugin-svelte, vite). The prettier bump reformatted four
  pre-existing files (whitespace only, no logic change).
- **CI action bumps**: `actions/checkout` 6→7, `actions/setup-go` 6.4→6.5,
  `golangci/golangci-lint-action` digest.

### Removed

- **Dead frontend code** flagged by ponytail-audit (grep-verified zero
  references, no behaviour change): the unused `ProgressBar` `label` prop and
  its branches/CSS, the `supportsWebSocket()` placeholder, and the orphaned
  `FirmwareUpdateResult` type.

## [0.5.4] - 2026-07-01 — Shelly 2.0.0-beta3: alt-firmware + provisioning visibility

### Added

- **Alternative firmware variants (`sys.alt`) surfaced read-only** — Shelly
  firmware 2.0.0-beta3 advertises alternative firmware builds for the same
  hardware (e.g. Power Strip Gen4 / Mini 1PM Gen4 → Zigbee, Pro 3EM → Pro
  Sensor Addon) under `Shelly.GetStatus` → `sys.alt`. ShellyAdmin now derives
  these from the already-cached `RawStatus` (no extra RPC, no new DB column,
  mirroring the `switch_count` deriver) into `Device.FWAlt`, exposed on
  `/api/devices`, MCP `get_device`/`list_devices`, and as an `alt: <id>` badge
  on the Firmware page's Model cell. **Visibility only** — `Shelly.Update` has
  no `stage:"alt"` and the alt object carries no `url`, so switching a device
  to a variant (e.g. flashing a plug to Zigbee firmware) is not possible via
  the documented API and is done from the device's own web UI. The
  `sys.provisioning` object (secure-provisioning state) rides the same read
  into `Device.Provisioning`, surfaced in `get_device`.

## [0.5.3] - 2026-06-10

Hardening release — items 1–8 from the June 2026 architecture/security
review (PRs #63, #64 + the rotate-key work).

### Added

- **`shellyctl rotate-key`** — encryption-key rotation as a one-shot
  subcommand. Re-seals every secretbox-encrypted value (device credentials,
  credential groups, TOTP secret + backup codes, the persisted MCP token)
  from the current key to a new one in a single transaction. Keys come from
  the environment (`SHELLYADMIN_ENCRYPTION_KEY[_FILE]` for the current key,
  `SHELLYADMIN_NEW_ENCRYPTION_KEY[_FILE]` for the new one), never argv.
  Without `--force` it is a dry run that verifies the old key opens every
  blob; with `--force` a timestamped DB backup is written first. Refuses to
  run while a server instance holds a fresh runtime-lock heartbeat. This
  replaces the manual clear-everything rotation playbook.
- **Template section validation at save time** — unknown top-level template
  keys (e.g. the typo `"syss"`) are rejected when the template is saved,
  with an error naming the valid sections and the `gen2_rpc` escape hatch.
  Previously they fell through to the `<Capitalized>.SetConfig` catch-all
  and only surfaced as a silent "skipped" section on every device during a
  provision run. Stored templates provision unchanged.
- **Frontend unit-test coverage gate** — vitest runs with
  `@vitest/coverage-v8` and a 30% statements/lines floor (measured
  ~36%/38% at introduction), counting every `src/**/*.ts` file whether a
  test imports it or not. CI's Frontend-build check enforces it.

### Changed

- **shellyclient hardening** — device response bodies are capped at 4 MiB
  (a misbehaving or hostile LAN endpoint can no longer OOM the
  scan/refresh workers with an unbounded response), and JSON-RPC response
  envelopes are validated: garbage or empty bodies on a success status now
  return an explicit error instead of a silent empty result, and an
  envelope carrying both `result` and `error` is rejected as malformed.
- **CI runs the Go test suite with `-race`** — lock-discipline regressions
  in the mutex-guarded service layer now fail the build.
- **`internal/db` split by domain** — db.go shrank from 1282 to 131 lines
  (connection lifecycle, migrations, helpers); queries moved verbatim into
  one file per table family. No behavior change.

### Documentation

- **SECURITY.md** — new "MCP Listener Token Hygiene" section (the
  path-segment auth form writes the token into anything that logs request
  paths; prefer the Authorization header, rotate on suspected exposure).
  "Encryption at Rest" rewritten: documents the mandatory external key
  (the section still described the pre-v0.3.0 auto-generated fallback),
  names every sealed surface, adds key-backup guidance and the
  `shellyctl rotate-key` procedure. Stale claims fixed (PATs/TOTP exist
  since v0.3.0).

## [0.5.2] - 2026-05-29

### Changed

- **Scan validation refactor + regression tests** — extracted a focused
  `validation.ScanParams` that validates only the scan-relevant fields
  (subnets, timeouts, concurrency) and returns the target count. `StartScan`
  now calls it instead of clearing the MCP fields on a copy before validation,
  removing that one-off special case and the duplicate CIDR-expansion /
  target-count it recomputed. `validation.Settings` delegates its scan portion
  to the same helper, so save-time validation (including the MCP token format
  check) is unchanged. Adds regression coverage for the v0.5.1 fix — a sealed
  (ciphertext) MCP token in the raw DB row no longer blocks a scan. No
  behavior change.

## [0.5.1] - 2026-05-24

### Fixed

- **Scan blocked when MCP is enabled** — starting a scan would fail with
  "mcp token must match [A-Za-z0-9_-]{16,128}" if an MCP token was
  configured. `StartScan` reads the raw DB settings row, whose `MCPToken`
  field is secretbox-encrypted ciphertext (not plaintext). The format
  validator was being called against the ciphertext, which always fails
  the URL-safe alphabet check. The token is now cleared from the local
  copy before validation; the scan-parameter checks (subnets, timeouts,
  concurrency) are unaffected. The token itself was already validated at
  save time.

## [0.5.0] - 2026-05-23 — first public release

Repo flipped from private to public on 2026-05-23. This release captures
the going-public hygiene pass; there are **no behavior changes** — every
diff is docs, build, or test fixtures. Existing v0.4.0 deployments
should upgrade for the polish but will be functionally identical.

### Added

- **`CODE_OF_CONDUCT.md`** — Contributor Covenant v2.1 pointer with
  GitHub Security Advisories as the reporting channel. Closes the last
  gap in GitHub's community profile checklist.
- **`.dockerignore`** — scopes local `docker build` runs away from
  maintainer-side working-tree state (`data/`, `.devlogs/`, `.claude/`,
  `bin/`, `secrets/`, env files, rebuilt SPA artifacts). Published GHCR
  images were never affected (CI builds via `actions/checkout`); this
  hardens local rebuilds.
- **`.gitattributes`** — marks `cmd/shellyctl/dist/**` as
  `linguist-generated` and `-diff` so the embedded SPA bundle doesn't
  pollute GitHub's language stats or diff view. Also pins LF line
  endings for `*.sh` and `Dockerfile`.
- **README badges** — CI status, MIT license, latest release, GHCR
  package, Go Report Card (currently **A+**).
- **README screenshots** — Devices view as hero, plus a collapsed
  Scan / Firmware / Provision / Compliance gallery in
  `docs/screenshots/`.
- **README "Why ShellyAdmin?" paragraph** — positions the tool
  relative to the Shelly cloud and Home Assistant.
- **Private Vulnerability Reporting**, **secret scanning**, and
  **push protection** enabled at the GitHub level once the repo was
  public.

### Changed

- **Scrubbed personal hostnames and tool names** from every tracked
  file. Replacements: `docker.home.lan` → `<docker-host>`,
  `/docker/shellyadmin` → `<data-dir>`, `mqtt.home.lan` →
  `mqtt.example.test` (RFC 6761 reserved test TLD), `buliwyf_iot` →
  `iot_wifi`, "Dockhand" → "container manager". Covers CHANGELOG
  history, deploy docs, ADRs, plan docs, scripts, and test fixtures.
- **`CLAUDE.md` slimmed by ~25 %** (369 → 276 lines). The Deployment
  Workflow, Release Cadence Convention, and CI Gates & Branch
  Protection sections moved to [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md);
  the file kept its 14 code-adjacent sections (architecture,
  Shelly quirks, key files, provisioner template surface, job
  locking, compliance, testability seams, app settings, first-run
  setup, encryption-key requirement, runtime locks, TOTP / PAT).
- **README rewritten** for the public landing page: deduplicated
  "What Works Today" + "Not Production-Grade Yet" (covered by Status
  + Feature Set above), consolidated "Running Locally" into a
  single two-terminal flow, tightened the "Docker" section, bumped
  the stale `v0.0.6` example tag, removed the internal
  `shellyctl` vs `ShellyAdmin` naming note.
- **`docs/plans/README.md`** reframed as archival: Phase 4b,
  Phase 4c, and v0.3.0 plans moved from "Active" to "Shipped";
  pointer to the maintainer's private originating plan removed.

### Removed

- **Stale root `docker-compose.yml`** (pinned to `v0.1.19`). The
  canonical example is [`docker/docker-compose.yml`](docker/docker-compose.yml),
  which uses `:latest`.

### Verification

- Full local CI green: `go test ./...`, `golangci-lint run ./...` (0
  issues), `go run ./cmd/modelschema --check`, `npm test` (81 / 81),
  `npm run lint`, `npx prettier --check src`,
  `npm run check:bundle-size` (js 358.23 / 365 kB, css 29.26 / 30 kB
  — under budget).
- All 6 required CI checks green on each of the seven going-public
  commits before this release commit.
- Git history scanned end-to-end for any leaked `.env`, key, db, or
  credential artefact — none found, history is clean.
- Anonymous `docker pull ghcr.io/buliwyf42/shellyadmin:v0.4.0`
  verified to succeed against the now-public GHCR package
  (1609-byte OCI image index returned with the proper `Accept`
  header).

## [0.4.0] - 2026-05-20 — first-run setup: operator login in the database

The operator login moves out of environment variables and into the database.
A fresh instance now boots into a **first-run setup** screen where you create
the admin account in the web UI — no more `shellyctl hash-password` +
hand-edited `.env` before the server will start. Existing deployments upgrade
seamlessly: a still-present `SHELLYADMIN_PASS_HASH` is imported into the
database once at boot, then ignored. See [ADR-0017](docs/adr/0017-first-run-setup.md).

### Added

- **First-run setup (ADR-0017).** New `admin_credentials` table (migration
  031) holds the operator username + argon2id hash. Public, rate-limited,
  one-shot `GET /api/setup/status` + `POST /api/setup` drive the setup screen
  (`/setup`); the SPA routes there automatically until an account exists.
- **Change credentials in Settings.** `POST /api/account/credentials`
  (authenticated, cookie-only) verifies the current password, updates the
  username/password, and revokes all sessions. New "Operator Account" card on
  the Settings page.
- **`shellyctl reset-auth --force`** clears the stored login, returning the
  instance to setup mode on the next boot — the forgotten-password recovery
  path, mirroring `shellyctl unlock`.

### Changed

- **`SHELLYADMIN_PASS_HASH` / `SHELLYADMIN_USER` are no longer required.** The
  startup panic on a missing hash is gone; with no credential the server boots
  into setup mode. The env hash is demoted to an optional one-time import seed.
  README, `docs/DEPLOYMENT.md`, and both compose files updated accordingly; the
  root `docker-compose.yml` no longer hard-fails when the hash is unset.
- The login handler resolves the credential from the database at request time
  (with an env fallback retained for tests); lockout and TOTP keys follow the
  resolved username.

## [0.3.6] - 2026-05-20 — shellyctl CLI, E2E + unit tests, deploy/test tooling

Tooling and test-coverage release. The one operator-facing addition is the
read-only `shellyctl` CLI; the rest hardens testing and the deploy workflow.
No change to the running server's behavior.

### Added

- **`shellyctl` read-only CLI (ADR-0016).** Queries a running instance over
  `/api` with a Personal Access Token: `devices` (list), `device
  <mac|ip|name>` (detail), `logs` (audit tail, with `--level/--search/--risk`).
  Human-aligned tables by default, `--json` for raw payloads; `--url` /
  `--token` or `SHELLYADMIN_URL` / `SHELLYADMIN_TOKEN`. Bare `shellyctl`
  still starts the server. Read-only first, mirroring the MCP server's
  staging (ADR-0011) — no new auth or data-access surface.
- **Playwright E2E (roadmap T8).** Smoke specs for login + the responsive
  nav (desktop bar, mobile hamburger drawer, version badge), plus a
  non-required `E2E (Playwright)` CI job that builds the SPA into the binary,
  boots it, and drives a real browser against it.
- **Pre-deploy DB snapshot tooling.** `scripts/snapshot-prod-db.sh`
  (host-side SSH copy, since a read-only container exec can't write the
  snapshot) + a documented pre-deploy step in `docs/DEPLOYMENT.md`.

### Changed

- Extracted the navbar's pure logic into `web/src/components/navbar.ts`
  (version-badge normalization + active-link detection) with unit tests,
  mirroring the `sortHeader.ts` pattern.
- Docs now use repo-relative markdown links instead of absolute local
  filesystem paths that only resolved on one machine.

## [0.3.5] - 2026-05-20 — Responsive nav + a11y, table-overflow fixes, CI image-build gate

Frontend responsiveness/accessibility pass plus CI hardening. No backend
behavior change.

### Added

- **Docker image build CI gate.** `test.yml` now smoke-builds
  `docker/Dockerfile` (single-platform amd64, no push) on every PR and
  push, and it's a required check on `main`. Closes the gap where a
  breaking base-image bump only surfaced 17-22 min into
  `publish-image.yml` at release time.

### Changed

- **Collapsible mobile navigation.** The 10-link top bar wrapped 3-4
  rows below ~1024px (~380px of vertical space eaten on mobile). It now
  collapses into a hamburger-toggled vertical drawer under 1024px; the
  horizontal bar is unchanged at >=1024px.
- **Accessibility baseline.** Nav toggle carries
  `aria-label`/`aria-expanded`/`aria-controls`; active links get
  `aria-current="page"`. Added a keyboard-only `:focus-visible` ring
  (0-specificity `:where()` so component styles still win) and a
  `prefers-reduced-motion` rule.
- **CI toolchains aligned to the Dockerfile.** `setup-go` 1.25 → 1.26
  and `setup-node` 20 → 26 (the `go.mod` floor stays `go 1.25.0`);
  `golangci-lint` bumped to v2.12 (v2.6 panics on the Go 1.26 stdlib).
- Docs refreshed for v0.3.4, the CI automation, and the new toolchains.

### Fixed

- **Narrow-viewport table overflow.** The Logs and Firmware tables were
  bare `<table>`s with no scroll wrapper, forcing document-level
  horizontal scroll on mobile. Both now use `.table-responsive`.
- **Badge contrast (WCAG AA).** Small badge labels with white text on
  `--danger` (3.94:1) and `--success` (2.71:1) were below the 4.5:1
  threshold; darkened to `#c93544` (5.15:1) and `#18804a` (4.97:1).
  Both tokens are consumed only by `.bg-danger`/`.bg-success`.
- **Navbar version badge** rendered `vv0.3.4` when the backend version
  already carried a leading `v`; the duplicate is now stripped.
- **`make dev-backend`** still set `SHELLYADMIN_PASS` (removed v0.2.0)
  and no `SHELLYADMIN_ENCRYPTION_KEY` (mandatory v0.3.0) and panicked at
  startup; it now derives the hash from a dev password and sets a fixed
  dev-only key (login: `admin` / `dev-secret`).

## [0.3.4] - 2026-05-20 — Clear-Logs trigger fix + Dependabot grouping/auto-merge

Maintenance release. Fixes the audit-log "Clear Logs" button (broken
since the S2 append-only trigger landed) and bundles the CI automation
and dependency bumps that accumulated since v0.3.3.

### Fixed

- **"Clear Logs" button no longer 500s.** `db.ClearLogs()` ran a naked
  `DELETE FROM audit_log`, which the S2 `audit_log_no_delete` trigger
  rejects as append-only — the SPA's `DELETE /api/logs` surfaced a 500
  and the table was never cleared. It now wraps the delete in the same
  `__retention_bypass` transaction pattern as `PruneAuditLogOlderThan`,
  flipping the flag inside the transaction so a crash mid-delete leaves
  the protection intact. Regression test `TestClearLogsRespectsBypass`.

### Changed

- **Dependabot updates are now grouped** (npm dev/prod, gomod, docker,
  github-actions) so the weekly run opens a handful of PRs instead of
  one per dependency. This also fixes the `@typescript-eslint`
  peer-dependency lockstep that broke isolated bumps (e.g.
  `eslint-plugin` 8.59.4 vs `parser` 8.59.3 `ERESOLVE`).
- **Dependabot auto-merge for patch + minor bumps.** A new
  `dependabot-auto-merge.yml` workflow enables GitHub auto-merge on
  patch/minor Dependabot PRs; the merge only fires after the required
  Test checks pass, so CI is never bypassed. Major bumps stay manual.
  Requires the `main` branch protection rule (5 required checks) added
  alongside this release.
- **Docker base images bumped**: `node` 20-alpine → 26-alpine
  (frontend build stage) and `golang` 1.25-alpine → 1.26-alpine
  (backend build stage). The `go.mod` directive stays `go 1.25.0`;
  the 1.26 toolchain builds it backward-compatibly. Validated with a
  local multi-stage image build.
- Dependency bumps: `svelte` 5.55.8, the npm dev-dependency group
  (`eslint`, `typescript-eslint` family, `vite`), and
  `sigstore/cosign-installer` 3.9.1 → 4.1.2. The cosign-installer v4
  major still installs the pinned cosign v2.4.1, so image signing is
  unchanged.

## [0.3.3] - 2026-05-12 — runtime_locks: same-hostname fast path + 60 s window

Hotfix patch. Softens ADR-0015 (`runtime_locks`) to recover gracefully
from the common-case **single-container crash-restart loop** that bit
the first production v0.3.0 deploy. Recommended upgrade for everyone
on v0.3.0–0.3.2.

### Fixed

- **Same-container restart no longer requires `shellyctl unlock
  --force`.** v0.3.0–0.3.2's lock policy was designed for the "two
  containers pointing at the same SQLite" misconfiguration, but it
  also punished the much more common "Docker `restart:
  unless-stopped` cycling a crashed container" case. When the
  previous boot died without releasing (SIGKILL, OOM, panic before
  the `defer Release` registered) the restarting container would
  read its own previous row, treat it as a foreign lock, and panic
  for 5 minutes until the staleness window expired. v0.3.3 adds a
  hostname fast path in `runtimelock.Service.Acquire`: when the
  existing row's hostname matches the current process's, take over
  immediately. Docker sets each container's hostname to the
  container ID prefix, so two genuinely-different containers have
  different hostnames by default — the fast path covers the
  crash-restart case without weakening the original protection.
- **StaleAfter window tightened from 5 min → 60 s.** Backstop for
  the residual case where the operator explicitly sets
  `hostname: shellyadmin` in two compose stacks pointing at the
  same DB (contrived but possible). 60 s is still long enough to
  catch a genuine two-instance misconfiguration — it takes >60 s
  of deliberate operator action to start two containers — without
  forcing a 5-minute wait when something legitimately crashed.

### Recovery path for operators on v0.3.0–0.3.2 stuck in the lock loop

Until v0.3.3 image is pulled, use the one-shot escape:

```
docker run --rm -v <data-dir>:/data -e DATA_DIR=/data \
  ghcr.io/buliwyf42/shellyadmin:v0.3.3 \
  unlock --force
```

(Note the absence of a leading `shellyctl` — the image ENTRYPOINT
is the docker-entrypoint.sh wrapper which already invokes shellyctl
with the passed args.) Then `docker compose start shellyadmin`.

## [0.3.2] - 2026-05-12 — TOTP QR code + dependabot housekeeping

Patch release. **One user-visible change** (TOTP enrollment now shows
a scannable QR code) plus a handful of dev-dep + CI-action bumps.
Operators on v0.3.0 / v0.3.1 with no TOTP enrollment in flight have
no functional reason to upgrade.

### Added

- **TOTP enrollment QR code.** The Settings 2FA card draws the
  freshly-minted otpauth:// URI into a 220 px `<canvas>` via the
  `qrcode` library (~24 KB raw / ~11 KB gzip). Operators can now
  scan with a phone-based authenticator (Google Authenticator,
  1Password, Authy, Bitwarden, etc.) instead of typing the base32
  secret by hand. Manual-entry secret + the raw otpauth URI move
  into a collapsed `<details>` block as the fallback for desktop
  password vaults that don't scan. The previously-deferred T1
  polish item is now done.

### Changed

- **Bundle-size budget** raised 345/92 → 365/100 KB raw/gzip to
  absorb the qrcode encoder. Measured 351.72 KB raw / 95.24 KB gzip.
- **Dev-dependency bumps** (closes 3 dependabot PRs):
  - `vitest` 4.1.5 → 4.1.6 (#16)
  - `@vitest/ui` 4.1.5 → 4.1.6 (#17)
  - `prettier-plugin-svelte` 3.5.1 → 3.5.2 (#20)
- **CI action bumps** in `.github/workflows/publish-image.yml`
  (closes 2 dependabot PRs):
  - `sigstore/cosign-installer` (#14)
  - `aquasecurity/trivy-action` (#15)

### Fixed

- The "pre-existing a11y warning at
  `web/src/pages/provision/WebhooksForm.svelte:97`" mentioned in the
  v0.3.0 release notes was actually fixed earlier (likely during the
  M2 frontend split in v0.2.14) — verified all 5 `<label>` elements
  have proper `for=` attributes. Stale memory note cleared.

## [0.3.1] - 2026-05-12 — Post-v0.3.0 CI hygiene + dependabot housekeeping

Patch release. **No runtime behavior change** vs. v0.3.0 — the image
built from this tag is functionally identical to `:v0.3.0`. Operators
already on v0.3.0 have no reason to upgrade beyond getting a green
CI badge on the deployed commit.

Cut to clear three post-tag failures on the v0.3.0 commit's Test
workflow, then to land a coordinated dependabot bump:

### Fixed

- **`internal/models/schema.gen.json` drift** (commit a833f9d).
  Missed step 2 of the v0.3.0 pre-flight (regenerate model schema).
  The TOTPRequired field added to AppSettings in T1 caused the M3
  drift-check gate to fail. Regenerated; the schema now reflects
  the live struct. Pure CI artifact — not read at runtime.
- **Coverage gate** (commit 6e899d7). The M7 services-split in
  v0.2.14 moved code into sub-packages whose tests stayed in the
  parent `internal/services` package; with the default package-local
  `-coverprofile` those sub-packages registered as 0% covered even
  though integration tests fully exercised them. Switched CI to
  `go test -coverpkg=./...` for an honest cross-package count
  (`38.5% → 51.1%` durable baseline) and bumped the floor from 40%
  to 45%. Added 17 package-local middleware tests so middleware
  itself reads 44.9% directly.
- **Coordinated dependabot bump** (commit 3835d32). Three separate
  Dependabot PRs for `@typescript-eslint/*` 8.59.2 → 8.59.3 each
  failed `npm install` because the peer-dep contract requires all
  three to land together. Bundled them into one local bump; the
  three Dependabot PRs will auto-close when this lands.

## [0.3.0] - 2026-05-12 — Phase 4c (TOTP 2FA + PAT bearer tokens) + breaking auth hardening

The Phase 4c half of the consolidated review — the operator-facing auth
strategics. Two new auth surfaces (TOTP 2FA + Personal Access Tokens),
plus two breaking changes that close long-standing deprecation windows
(encryption-key auto-generation removed; single-instance-only enforced).

Operators upgrading from v0.2.x **must** perform a one-time migration
of the encryption key before starting v0.3.0. See "Breaking changes"
below for the recipe.

### Added

- **TOTP 2FA (T1, Block 4c.1)** — operator self-service second factor.
  RFC 6238 stdlib implementation (`internal/services/totp/totp.go`)
  plus orchestration in `internal/services/totp/service.go`. Login
  handler grows an optional `totp_code` field; missing → 401
  `totp_required` (no lockout bump, mid-flow), wrong → 401
  `invalid_totp_code` (bumps the per-account lockout counter so
  brute-forcing the 6-digit code is bounded by the same budget as
  password retries). Ten single-use backup codes issued at enrollment;
  reuse of a burned code returns the same shape as a never-existed one
  (no enumeration oracle). Four new endpoints under `/api/totp/*`
  (status, enroll, verify-enroll, disable). SPA two-step login prompt
  + Settings card.
- **Personal Access Tokens (T3, Block 4c.2)** — bearer-token
  credentials for headless callers (Home Assistant, cron jobs, scripts)
  so `/api/*` mutations no longer require faking the cookie + CSRF
  dance. Token format `pat_<8hex id>_<64hex random>` (256 bits of
  CSPRNG entropy in the random component; sha256+ConstantTimeCompare
  for the stored hash — argon2id would burn 80ms per PAT request for
  no security gain over a 256-bit secret). Eight-scope catalog
  (`admin`, `devices:read/write`, `firmware:read/write`, `provision`,
  `settings:read/write`); per-route scope enforcement via
  `middleware.RequireScope`. Bearer-authed requests skip CSRF (the
  token IS the proof-of-intent). PAT-authed callers cannot mint or
  revoke other PATs (privilege-escalation guard at the handler). Three
  new endpoints under `/api/tokens` (list, create, revoke) + Settings
  card.
- **`shellyctl unlock --force` subcommand** — manual recovery for the
  ADR-0015 single-instance lock when an operator knows the previous
  container died and doesn't want to wait the 5-minute staleness window.
- **18 new tests** — 9 in `internal/services/totp/service_test.go`,
  11 in `internal/services/tokens/tokens_test.go`, 6 in
  `internal/api/handler_login_totp_test.go`, 7 in
  `internal/api/handler_tokens_test.go`, 7 in
  `internal/services/runtimelock/runtimelock_test.go`.

### Changed

- **Per-route scope authorization (T3)** — every authenticated
  `/api/*` route declares a `Scope` on its `apiRouteDoc` entry;
  `registerDocumentedAPIRoutes` wraps the handler in
  `middleware.RequireScope(route.Scope)` with `admin` as the
  default-deny scope. Cookie-authed callers pass through unchanged;
  PAT-authed callers without the required scope (or `admin`) get a
  403 with `required_scope` in the body.
- **Bundle-size budget** raised from 320/86 KB (raw/gzip JS) to
  345/92 KB to absorb the new TOTPCard + TokensCard.

### Breaking changes

- **S6 — Encryption-key auto-generation removed (ADR-0013).** v0.2.11
  added a deprecation warning when no `SHELLYADMIN_ENCRYPTION_KEY` /
  `_FILE` was set and the service fell back to auto-generating a key
  at `{dataDir}/shellyadmin.key`. v0.3.0 closes the window: the boot
  refuses to start without an external key. Operators must:

  1. `docker exec shellyadmin cat /data/shellyadmin.key` to retrieve
     the existing key.
  2. Move it to a path outside the data volume (Docker secret, NixOS
     secret store, sops-encrypted file in the homelab config repo).
  3. Add `SHELLYADMIN_ENCRYPTION_KEY_FILE=/run/secrets/shellyadmin_encryption_key`
     (or wherever you placed it) to compose `.env`.
  4. Pull v0.3.0, recreate stack.

  Threat closed: a volume snapshot exfiltrating both the encrypted
  credentials in `shellyctl.db` AND the key file sitting next to it
  defeated the at-rest encryption. External key management means both
  halves no longer share a backup boundary.

  Skipped migration → clear startup error pointing at the legacy path
  + this recipe. Container exits with non-zero status; SQLite is
  untouched.

- **ADR-0015 — Single-instance-only enforced.** New
  `030_runtime_locks.sql` migration + `internal/services/runtimelock`
  package. On startup the service writes the `primary` row, runs a
  60s heartbeat, and releases on graceful shutdown. A second container
  starting against the same SQLite file finds a fresh row and refuses
  to boot — the error names the foreign hostname / pid / acquired_at
  + when the row will go stale. A stale row (5+ minutes without
  heartbeat — e.g. previous container was `kill -9`'d) is silently
  overwritten.

  Why: process-local state (login rate-limit map, MCP listener,
  background workers) doesn't replicate across instances. Two
  containers would double-spawn the firmware-check scheduler, race
  the audit-log retention transaction, and try to re-bind `:8081`.

  Operators rolling a new container by stopping the old + starting
  the new without `docker compose down` first will hit a startup
  error for up to 5 minutes. `shellyctl unlock --force` clears the
  row manually for the operator who knows the previous container is
  truly dead and doesn't want to wait.

### Migration checklist

1. Pre-pull a v0.2.x SQLite snapshot for rollback safety:
   `cp <data-dir>/shellyctl.db <data-dir>/shellyctl.db.pre-v0.3.0-$(date +%s)`.
2. Apply the S6 migration recipe above. Don't delete
   `<data-dir>/shellyadmin.key` yet — it's a rollback aid.
3. `docker compose down shellyadmin` (clean shutdown so the
   runtime_locks row is released before the upgrade).
4. Pull `ghcr.io/buliwyf42/shellyadmin:v0.3.0` (or `:latest`).
5. Recreate the stack with the updated `.env`.
6. Verify `/ready` returns `version: "0.3.0"` and the Settings page
   shows the new "Two-Factor Authentication" + "Personal Access
   Tokens" cards.

### Rollback

The migration is one-way at the encryption-key level (the operator
copied the key out; it still exists on the data volume unless deleted
manually). To roll back to v0.2.x: pull the previous image tag and
recreate the stack without `SHELLYADMIN_ENCRYPTION_KEY_FILE` — v0.2.x
finds the original `/data/shellyadmin.key` and continues. The two new
DB tables (`totp_state` migration 028 already shipped in v0.2.14
foundation work; `personal_access_tokens` migration 029 in v0.3.0;
`runtime_locks` migration 030 in v0.3.0) are additive — v0.2.x simply
doesn't read them.

Operators who deleted `/data/shellyadmin.key` after the migration
cannot roll back without restoring it from their key-management store.

## [0.2.14] - 2026-05-11 — Phase 4b (services split + Device payload + CSP hardening)

Phase 4b from the consolidated review — the architectural refactor block
(M7 services internal split, M8 Device list-view payload reduction, M2
frontend page split, M6 CSP hardening). No DB migration, no operator-
visible breaking change. Drop-in upgrade from v0.2.13.

The breaking v0.3.0 cut (Phase 4c — TOTP 2FA, Personal Access Tokens,
encryption-key externalization, runtime_locks enforcement) is the next
release window; this one lands the foundation work without forcing the
operator-side migration.

### Added

- **`models.DeviceListView`** (M8) — slim `/api/devices` projection
  dropping the 5 list-page-unused fields (`supported_methods`, `batch`,
  `fw_id`, `consecutive_misses`, `mqtt_flags_na`). The full `models.Device`
  still returns from `/api/devices/{target}` and the MCP `get_device`
  tool. Measured payload reduction: **49.2 %** on a synthetic 50-device
  fleet (122 KB → 62 KB). Regression-pinned by
  `internal/models/payload_bench_test.go` at >= 30 % reduction.
- **Eleven new sub-packages** under `internal/services/` (M7):
  `sessions/`, `mcp/`, `credentials/`, `jobs/` (with `refresh.go`,
  `scan.go`, `firmware_check.go`, `firmware_install.go`, `recovery.go`,
  `service.go`, `types.go` + types_test.go), `validation/`, `backup/`,
  `loginlock/`, `workers/`, `provisioning/`, `templates/`, `logs/`,
  `audit/`, `settings/`. Each has a narrow `Store` interface;
  AppService keeps delegators on every public method so existing
  callers (api/, mcp/, cmd/shellyctl/main.go, tests) compile unchanged.
- **Ten new Svelte child components** (M2):
  `pages/devices/{ColumnPicker,DevicesToolbar,DeviceTable}.svelte`,
  `pages/compliance/{ComplianceRulesForm,CustomRulesList,DeviceMatrix}.svelte`,
  `pages/provision/{TemplatesPanel,IPListPanel,ResultsPanel}.svelte`,
  plus `lib/deviceFormatters.{ts,test.ts}` (15 new pure-function tests).
- **Eight new utility CSS classes** in `web/src/app.css` (M6):
  `.text-hint-xs/sm/md/lg`, `.text-tiny-inline`, `.login-card-width`,
  `.mw-22r`, `.badge-restart-required`. Replace all 21 inline `style="..."`
  attributes the SPA was emitting at runtime.
- **`internal/util/secrets.go`** — `DecodeSecretValue` moved out of
  services package; `cmd/shellyctl/main.go` keeps using
  `services.DecodeSecretValue` via re-export alias.

### Changed

- **CSP `style-src` tightened to `'self'`** (M6) — drops `'unsafe-inline'`
  from the SPA's `Content-Security-Policy` header. Future DOM-injection
  sinks now fail closed at the style-attribute boundary; the browser
  rejects the injected inline style before the page repaints.
- **`internal/services/app.go` shrunk from 1149 → 436 LOC** (-62 %) as
  the sub-package extraction lifted out validation, backup, loginlock,
  workers, provisioning, templates, logs, audit, settings, the job
  bodies + types, the MCP listener controller, and the sessions
  surface. The public API of `*AppService` is unchanged — every former
  method is a one-line delegator.
- **`internal/services/app_jobs.go` shrunk from 1098 → 213 LOC** (-81 %).
  All long-running goroutines (`runRefreshJob`, `runScanJob`,
  `runFirmwareJob`, `runFirmwareInstallJob`, `installOne`,
  `runFirmwareCheckScheduler`, `RecoverInterruptedJobs`) move to
  `internal/services/jobs/` with a Store + Host interface split:
  Store is the raw DB-row surface, Host is the runtime/concurrency/RPC-
  factory surface `*AppService` provides.
- **Devices.svelte: 1098 → 258 LOC** (-76 %). The table renders via
  `DeviceTable.svelte`, the toolbar via `DevicesToolbar.svelte`, the
  column-visibility picker via `ColumnPicker.svelte`. Pure helpers
  (sort comparator, badge classes, refresh-state text, lat/lon format,
  generation label) live in `lib/deviceFormatters.ts` with vitest
  coverage.
- **Compliance.svelte: 1141 → 91 LOC** (-92 %). The giant SectionCard
  form with its 30+ enable toggles + the
  `initToggles`/`applyTogglesToSettings`/`ensureDefaults` round-trip
  lifts into `ComplianceRulesForm.svelte`. The right-column summary +
  device-status table lives in `DeviceMatrix.svelte`. The custom-rules
  editor lives in `CustomRulesList.svelte`.
- **Provision.svelte: 1046 → 793 LOC** (-24 %). The post-provision
  results table moves to `ResultsPanel.svelte`, the device picker +
  precheck summary to `IPListPanel.svelte`, the template loader +
  save/rename/delete + credential select to `TemplatesPanel.svelte`.
  The per-section form binding stays on the parent because each of
  the 18 per-section state objects (SysState, MqttState, ...) is
  bound deeply by the existing `provision/*Form.svelte` children.
- **MCP `list_devices` returns the slim `DeviceListView`** shape;
  `get_device` still returns the full `services.DeviceDetail` (no
  breaking change for MCP consumers per the ADR-0011 list-vs-detail
  contract).

### Removed

- **`BoundedConcurrency`** function in services package — moved
  earlier to `internal/services/jobs/service.go` as the only caller;
  the services-level copy was dead code.
- **`sanitizeTags`** function in services package — duplicated into
  `credentials/` and `backup/` sub-packages (each has the same
  small body); services-level copy unused after the moves.
- **`MCPBuilder` / `MCPController`** as services-package types —
  re-exported as type aliases from `internal/services/mcp.{Builder,Controller}`
  so existing `cmd/shellyctl/main.go` + `app_mcp_test.go` callers
  compile unchanged.

### Test coverage

- **Vitest count grows 57 → 72** (+15 from
  `lib/deviceFormatters.test.ts`).
- **Go test count unchanged** at 17 packages — the sub-packages share
  the existing `internal/services` test suite via the AppService
  delegators; new sub-package-specific tests will land alongside
  follow-up extractions.

### Architecture notes

- The Store + Host interface split in `internal/services/jobs/` is
  the canonical pattern for future sub-package extractions. Store
  is "what to mutate" (DB rows); Host is "what to call back to the
  runtime" (concurrency state, RPC client factories, logger, metrics).
  Each sub-service composes the two narrowly so tests can substitute
  either half without touching the other.
- The frontend page split established
  `pages/<page>/<Child>.svelte` for UI islands +
  `lib/<page>Formatters.ts` for pure presentation helpers + vitest
  tests. Persisted Svelte stores (`colVis`, `firmwareChannel`) are
  read directly from children — no prop-drilling.

## [0.2.13] - 2026-05-11 — Phase 4a (consolidated review)

Phase 4a from the consolidated review — the low-risk, mostly-additive
slice of the Phase-4 backlog. Three new ADRs, an argon2id parameter
bump with backward-compatibility helper, an optional audit-webhook
sink, and the documentation halves of two larger items
(API-versioning policy, binary-signing roadmap) that need their own
release window when the code work lands.

No DB migration, no breaking change. Drop-in upgrade from v0.2.12.

### Added

- **Audit webhook sink** (T11) — new `AppSettings.AuditWebhookURL`
  + `AuditWebhookMinLevel` settings. When the URL is set, every
  audit_log row is POSTed as JSON to the operator-supplied
  endpoint on a fire-and-forget goroutine (5s timeout, no retry).
  The local `audit_log` row stays the authoritative trail; the
  webhook is the replica. Compact payload shape
  (`{ts, level, message, request_id, risk_level, source}`)
  consumable by Slack incoming-webhook formatters, Discord,
  Loki-push receivers, or any plain JSON sink. URL validated at
  SaveSettings time (rejects non-http(s) schemes, missing host,
  relative paths). Two new tests in
  `internal/services/audit_webhook_test.go`.
- **`services.IsLegacyParameters`** (T6) — reports whether a
  stored argon2id PHC hash uses parameters below the current
  OWASP-2025 floor (m=96MiB). Used at startup to warn the
  operator that their `SHELLYADMIN_PASS_HASH` should be
  regenerated. One new test
  `TestIsLegacyParameters`.
- **Three new ADRs** in `docs/adr/`:
  - `0013-encryption-key-externalization.md` — codifies the
    v0.2.11 → v0.3.0 hard-fail transition + migration recipe.
  - `0014-mcp-agent-threat-model.md` — names what ADR-0011
    assumed: a prompt-injected agent bypasses the confirm-gate.
    Three Phase-4-code defenses queued with acceptance criteria.
  - `0015-single-instance-constraint.md` — formalises why
    ShellyAdmin is single-instance-only + the `runtime_locks`
    detection table v0.3.0 will introduce.

### Changed

- **Argon2id parameters** bumped to OWASP-2025
  (m=64MiB → m=96MiB) in `internal/services/password.go` (T6).
  Existing PHC hashes with m=64MiB continue to verify; new hashes
  produced by `shellyctl hash-password` use the new floor.
  Login response time grows by ~15 ms; tests slowed by ~80 ms per
  argon2-invoking case (still well under the test timeout).
- **`docs/ARCHITECTURE.md`** new "API Versioning Policy" section
  (T7) — codifies the pre-v1.0 no-guarantee stance, the post-v1.0
  `/api/v1` stable / `/api/v2` breaking pattern, and the
  one-release-cycle deprecation window. Anchor for the actual
  prefix-mounting work queued for v0.3 → v1.0.
- **`docs/DEPLOYMENT.md`** new "Standalone Binary Distribution"
  section (T12) — documents that today's binary build is
  unsigned, lists the three concrete deliverables for a future
  signed-binary release (goreleaser config, cosign blob signing,
  SLSA-L3 provenance), and includes the operator-side verify
  command template.

### Tracking

- `cmd/modelschema` snapshot regenerated to include the two new
  `AppSettings` webhook fields. M3 drift check passes against
  the refreshed schema.

### Deferred to v0.3 (no change since v0.2.12)

- M7 (services internal split — XL refactor)
- M2/M6/M8 (frontend page split + Trusted-Types CSP +
  DeviceListView/Detail split — XL block)
- T1 TOTP 2FA, T3 PATs (XL each)
- T2 WebAuthn (XXL, post-T1)
- T4 HSM/PKCS11 (XXL)
- T8 Playwright E2E (L, post-M2)
- T9 external pen test (extern)
- S6 encryption-key hard-fail (planned breaking change at v0.3.0)

### Upgrade notes

- **No breaking change**, no DB migration.
- **New optional setting** `AuditWebhookURL` (empty default
  preserves current behaviour).
- **Startup warning** appears if `SHELLYADMIN_PASS_HASH` was
  generated with v0.2.x's m=64MiB parameters. Action:
  regenerate with `shellyctl hash-password <plaintext>`.
- **Encryption-key auto-generation deprecation warning** from
  v0.2.11 still fires; v0.3.0 will turn it into a hard-fail
  (see ADR-0013 for the migration recipe).

## [0.2.12] - 2026-05-11 — Phase 3 modernization (partial)

Phase 3 from the consolidated review. Ships 9 of 13 Phase-3 items
across architecture refactoring, drift-prevention CI gates,
observability, and operator documentation. The two remaining XL
items (M7 services-internal split, M2/M6/M8 frontend page split +
Trusted-Types CSP without `unsafe-inline` + DeviceListView/Detail
split) are deferred to v0.3 — each is its own multi-day refactor
with a wide test surface, and shipping the cheap items first means
operators get the wins without waiting on the long pole.

### Added

- **`/metrics` endpoint** (M4 + M10) — opt-in via
  `SHELLYADMIN_METRICS_BIND` (e.g. `127.0.0.1:9100`). Exposes
  `shellyadmin_devices_total` (gauge), `shellyadmin_refresh_jobs_total`,
  `shellyadmin_firmware_jobs_total`, `shellyadmin_audit_rows_written_total{level}`,
  `shellyadmin_http_requests_total` in Prometheus text exposition
  format (v0.0.4). New `internal/observability` package implements
  the registry stdlib-only — no `prometheus/client_golang` dep
  until the surface grows histograms. 5 tests cover the
  counter/gauge/labelled paths and label escaping.
- **Schema drift check** (M3) — `cmd/modelschema` emits a canonical
  JSON snapshot of the 8 SPA-serialised structs (AppSettings,
  ComplianceRules, CustomRule, Device, Credential, CredentialGroup,
  DeviceCredentialGroupAssignment, Job). New CI step
  `go run ./cmd/modelschema --check` fails the build when a Go
  struct field is added without regenerating the snapshot. Lite
  version of M3 — full Go→TS codegen on every change costs more
  than the current schema-evolution rate pays back; this lands the
  drift-prevention half.
- **OpenAPI route coverage test** (M9) — new
  `TestEveryAPIRouteIsDocumented` walks the live gin
  `Engine.Routes()` and fails if any `/api/`, `/health`, or
  `/ready` path is missing from `documentedAPIRoutes()`. Pairs
  with the existing `TestDocumentedAPIRoutesMatchExpectedRouteSet`
  for two-direction coverage.
- **Cloud-metadata SSRF deny** (M5) — `isProvisionTargetAllowed`
  explicitly rejects `169.254.169.254`. It sits inside the RFC3927
  link-local /16 so the previous `IsLinkLocalUnicast()` check let
  it through. Test case in `TestIsProvisionTargetAllowed`.
- **Off-host log forwarding docs** (M12) — new `docs/SECURITY.md`
  section with a Promtail config fragment for picking up the
  v0.2.11 stderr slog tee, plus three useful Grafana/Loki queries
  (lockout detector, daily high-risk count, request_id trace).
- **Network segmentation guide** (M13) — new `docs/SECURITY.md`
  section codifying the three-VLAN minimum (Admin / IoT / Guest)
  with OPNsense, UniFi, and pure-iptables examples. Reverse-proxy
  + `SHELLYADMIN_TRUSTED_PROXIES` posture documented.

### Changed

- **`handler.go` split** (M1) — the 772-line / 45-method monolith
  is now nine resource-specific files (`handler_auth.go`,
  `handler_devices.go`, `handler_scan_firmware.go`,
  `handler_provision.go`, `handler_settings.go`,
  `handler_templates.go`, `handler_credentials.go`,
  `handler_logs_backup.go`, `handler_meta.go`). `handler.go` keeps
  the Handler struct, NewHandler, audit-sink wiring, and the
  in-package helpers (logReq, emitSlogWithRisk, RandomSecret,
  decodeJSON). No semantic change.

### Tracking

- **Issue #13** opened for M11 (`go.mongodb.org/mongo-driver/v2`
  transitive dep via `gin/binding`). Not a quick-fix — gin
  upstream needs to split the BSON binding behind a build tag,
  or we pin to a fork. Deferred + tracked.

### Deferred to v0.3

- **M7** services internal split (XL refactor — `internal/services/`
  into `jobs/`, `mcp/`, `credentials/`, `backup/` sub-packages).
  AppService has 50+ methods crossing concerns; right-shaping them
  is a multi-day, wide-test-surface change worth its own release.
- **M2** frontend page split (Compliance, Devices, Provision each
  >1000 LOC).
- **M6** CSP without `unsafe-inline` (depends on M2 — Svelte
  component styles compile to inline `<style>` until those pages
  shrink).
- **M8** `models.Device` split into `DeviceListView` (slim) and
  `DeviceDetail` (full). Depends on the M3 codegen pipeline being
  extended into actual TypeScript generation, which is currently
  deferred.

### Upgrade notes

- **`SHELLYADMIN_METRICS_BIND`** is the new opt-in env var. Empty
  default = no metrics listener; existing deployments are
  unchanged. Pair with loopback + Prometheus host firewall rule;
  the endpoint itself is unauthenticated.
- **No DB migration** in this release — Phase 3 items are
  architecture + observability, not schema.

## [0.2.11] - 2026-05-11 — Phase 2 stabilization (consolidated review)

Phase 2 of the consolidated security + architecture review.
Implements 19 of 19 Phase-2 items (S1–S21). The headline change
is **server-side session revocation** (S5): a stolen session cookie
is no longer valid for 7 days after the operator clicks Logout.
The release also adds **cosign image signing** + **Trivy image
scan** to the publish pipeline, **audit-log hash-chaining** with an
append-only trigger, and **encryption-key externalization as a
soft-deprecation** (v0.3.0 will refuse to start without an external
key).

### Added

- **Server-side session store** (S5). New `sessions` table; Login
  issues a row, Logout flips `revoked_at`, RequireAuth consults the
  row on every request. Background sweeper prunes expired rows
  every 6 hours. 5 new tests cover the lifecycle, sweeper
  selectivity, bulk-revoke isolation, and the end-to-end
  Login → Logout → cookie-refused flow.
- **Cosign keyless signing** of the multi-arch image index in
  `publish-image.yml` (S3). Operator-side verify command
  documented in `docs/DEPLOYMENT.md`:
  `cosign verify --certificate-identity-regexp '...' ...`.
- **Trivy HIGH/CRITICAL scan** against the freshly-pushed digest
  (S4). Fails the workflow after the push, so the operator gets a
  notification before rolling production forward.
- **Audit-log hash chaining** (S2). New `prev_hash` column +
  `audit_log_no_delete` append-only trigger. Tampering with a row
  breaks the chain at the next link; `VerifyAuditChain` walks the
  table and reports the first mismatch.
- **Audit-log retention** (S1) — `AuditRetentionDays` setting
  (default 90, clamped [0, 3650]). Hourly background job prunes
  older rows via a controlled bypass of the append-only trigger.
- **Auto-backup snapshots** (S12+S13) — opt-in via
  `AutoBackupEnabled` setting. Hourly tick consults the operator-
  configured interval/keep policy and writes
  `shellyctl.db.snap-<UTC>.sqlite` via SQLite `VACUUM INTO`.
  Encryption key NOT snapshotted by design.
- **`/ready` endpoint** (S7) returns DB-ping latency + MCP
  listener status as JSON. Returns 503 when degraded. `/health`
  stays a flat 200/OK for container liveness probes.
- **MCP-listener rate-limit** (S8) — same token-bucket cadence as
  the SPA API (300/min/IP) but a separate counter store so the
  two surfaces don't starve each other.
- **TrustedProxies env var** (S11) — `SHELLYADMIN_TRUSTED_PROXIES`
  comma-separated CIDR list. Without it, `X-Forwarded-For` from
  any LAN peer was silently trusted by `ClientIP()`.
- **STRIDE Threat Model ADR-0012** (S19) — formalizes the
  trusted-LAN-is-hostile posture and maps each defense layer to
  STRIDE categories. Future security PRs reference this ADR for
  context.
- **CI version-sync gate** (S20) — fails the build if `VERSION`,
  `web/package.json`, and `web/package-lock.json` drift apart.
- **CI coverage gate** at >=40% (S15) — baseline 41.6% at this
  cut. Phase 3's handler.go split + services split will lift the
  ceiling toward 50%.

### Changed

- **SanitizeLogMessage regex** (S21) — now redacts JSON-form
  `"password":"..."` and URL-encoded `password=x&secret=y`
  patterns. Two real disclosure paths the 8-test regression
  suite uncovered. Extended secretPattern character class.
- **Firmware-check scheduler** (S9) restarts on panic with a
  <5s crash-loop guard. A bad SQLite tick no longer leaves the
  service silently without periodic checks.
- **Refresh-job spawn** (S10) — check-then-spawn now serialised
  under `jobSpawnMu` so concurrent `/api/refresh` calls cannot
  both pass the "already running" gate.
- **CSP** gains `require-trusted-types-for 'script'` (S17). The
  open Trusted-Types allowlist is deliberate — pinning 'none' or
  a specific factory would break Svelte 5's compiled output if
  it ever registers a default policy.
- **Dockerfile** initialises `/data` and `/tmp` as shelly-owned
  at build time (S14). Operators can opt into
  `user: <shelly-uid>` in compose and drop the CHOWN/SETGID/
  SETUID capabilities entirely; the entrypoint detects non-root
  and skips the privileged steps.
- **slog deprecation warning** when no external encryption key is
  provided (S6). v0.3.0 will turn this into a hard-fail. Two-
  version window gives operators time to migrate their `.env`
  to `SHELLYADMIN_ENCRYPTION_KEY_FILE`.
- **govulncheck** is now informational (`continue-on-error: true`).
  Stdlib CVEs land in the Go vuln DB before the corresponding
  patch release reaches setup-go's `1.25` alias; blocking CI on
  every such gap was friction without benefit. The output still
  appears in the job log and is part of the pre-tag review.

### Documentation

- **`docs/adr/0012-stride-threat-model.md`** — new ADR.
- **`docs/DEPLOYMENT.md`** — cosign verify command + Trivy notes.
- **`CLAUDE.md`** — documents that quic-go is linked but no UDP
  listener is opened (verified with `go tool nm`).
- **`docker/docker-compose.yml`** — cap_add comment explaining
  the path to dropping CHOWN/SETGID/SETUID.

### Security

Together with v0.2.10's Phase 1 work, this release closes the
three systemic risks the consolidated review identified:

1. **Supply-chain compromise** — SHA-pinned actions (v0.2.10) +
   cosign sign-and-verify + Trivy scan (v0.2.11). The operator
   has a cryptographic verification path; without it the CI
   signing would be theater.
2. **Cookie-theft persistence** — server-side session revocation
   (S5). The session can now be invalidated by the operator
   without waiting for the cookie's MaxAge.
3. **Audit-log tampering** — hash-chain + append-only trigger.
   A post-compromise admin cannot rewrite history without
   leaving a chain break the verifier reports.

Phase 3 (handler.go split, services split, frontend page split,
codegen models→types, `/metrics`, no-`unsafe-inline` CSP) is the
next workstream. Phase 4 (TOTP 2FA, external pen-test) follows.

### Upgrade notes

- **Active sessions from v0.2.10 are invalidated.** Operators
  will be redirected to `/login` on their next request after
  upgrading. A clean re-login is the migration path.
- **Operators wanting LAN-reachable MCP** must still set
  `SHELLYADMIN_MCP_BIND=0.0.0.0` in their `.env` (v0.2.10
  change carried forward).
- **Encryption-key auto-generation prints a deprecation warning**
  to stderr. Set `SHELLYADMIN_ENCRYPTION_KEY_FILE` to a file
  outside the data volume before v0.3.0.

## [0.2.10] - 2026-05-11 — Phase 1 security hardening (consolidated review)

Three-PR security-hardening sprint shipping the Phase 1 quick wins
from the consolidated architecture + security review. The review
abandoned the "trusted LAN" assumption — these changes harden the
SPA-cookie-session attack path and the supply chain against a
compromised IoT device on the same subnet. No breaking API
changes; one operational note for MCP-exposing operators.

### Added

- **`.github/dependabot.yml`** — weekly gomod + npm + GitHub-Actions,
  monthly Docker base-image updates. Closes the supply-chain
  blindspot the review flagged (E5).
- **`govulncheck ./...`** as a CI job in `test.yml` (Go-vulnerability
  database). Fails the build on known-vulnerable transitive deps.
- **`npm audit --omit=dev --audit-level=high`** as a CI step on the
  frontend job. Production-dep-only — devDeps are accepted upstream
  noise.
- **MCP-token format validation** at `SaveSettings` — restricted to
  `[A-Za-z0-9_-]{16,128}`. A `/` in the token would silently break
  the URL-path MCP auth form; other URL-reserved chars would need
  encoding the client may skip.
- **Account-lockout** (`internal/db/migrations/025_login_state.sql`
  + `services.AppService` + `internal/api/handler.go`): after 20
  consecutive failed logins the account locks for 15 min. State
  persists in SQLite, so killing the container does not reset it.
  Returns `423 Locked` + `Retry-After` header.
- **Snapshot test** for `GET /api/settings → MCPToken == "<set>"`
  to lock in the redaction contract.
- **Login regression tests** (6) — happy path, wrong password,
  wrong username, lockout-after-N, unlock-on-success, timing-
  flatness check that catches >20 ms gaps between wrong-user and
  wrong-password paths.

### Changed

- **All GitHub Actions SHA-pinned** in both `test.yml` and
  `publish-image.yml` with the tag preserved as a comment.
  Closes the `tj-actions/changed-files`-style maintainer-compromise
  window (E1 in the review).
- **Dockerfile base images digest-pinned** —
  `node:20-alpine@sha256:fb4cd1…`,
  `golang:1.25-alpine@sha256:8d22e2…`,
  `alpine:3.21@sha256:48b030…`. Docker Hub tag-pivot can no longer
  slip into a release build. Dependabot's `docker`-ecosystem will
  bump these weekly.
- **`npm ci --ignore-scripts`** in the frontend build stage —
  blocks malicious postinstall hooks in compromised npm deps.
- **SQLite DSN** now applies `_pragma=foreign_keys(on)`,
  `busy_timeout(5000)`, `journal_mode(WAL)` per-connection. Closes
  the silent FK-not-enforced gap that the review flagged (S2/S3) —
  migrations declare FKs but SQLite needs the pragma per
  connection. Write contention now backs off for 5 s instead of
  failing immediately.
- **`SHELLYADMIN_MCP_BIND` defaults to `127.0.0.1`** (was
  `0.0.0.0`). MCP-token-only auth warrants the tighter default.
  **Operator note:** compose stacks that map
  `:8101→:8081` from the host must now set
  `SHELLYADMIN_MCP_BIND=0.0.0.0` in the stack `.env`; otherwise
  the listener is unreachable.
- **Cookie `SameSite`** Lax → Strict. Cross-site top-level GETs
  no longer attach the session cookie. Following an external
  link into ShellyAdmin requires a fresh sign-in on that tab.
- **CSRF token no longer echoed** as the `X-CSRF-Token` response
  header on every authenticated GET. Previously a single XSS sink
  in the SPA could grab the token via
  `fetch('/api/devices').then(r => r.headers.get('X-CSRF-Token'))`.
  The token now flows only through the login response body and
  the dedicated `GET /api/csrf-token` endpoint. Frontend updated
  to match.
- **Login-handler timing-oracle closed** — argon2id verification
  always runs, even on username mismatch. Pre-fix the
  short-circuit `||` skipped the ~80 ms hash check on a wrong
  username, letting an attacker enumerate valid usernames by
  response timing.
- **slog now multi-writes** to the lumberjack file sink + stderr.
  Cluster log collectors (Loki, docker-logs, k8s log streams)
  finally see the structured JSON that previously lived only in
  the volume's `shellyctl.log`.
- **`gin.Default()` → `gin.New()` + custom `StructuredLogger`
  middleware**. Request lines flow through slog (same sink as
  audit), query strings are stripped from logged paths, request
  IDs are correlated.
- **`r.NoMethod`** returns JSON for `/api/*` paths instead of
  gin's default HTML 405 page, mirroring the existing
  `r.NoRoute` JSON-for-API behaviour.
- **`shellyctl hash-password <plain>`** emits a stderr warning
  about argv leaking via `ps`/shell history/container logs.
  Stdin form is unchanged and preferred.
- **`CLAUDE.md`** `SHELLYADMIN_USER`-drift fix — variable still
  exists and defaults to `admin`. Only `SHELLYADMIN_PASS`
  plaintext was removed in v0.2.0, not the user concept.
- **`.golangci.yml`** `go: "1.24" → "1.25"`. CI Go-version has
  been 1.25 since the v0.1.16 floor bump.

### Security

The review identified the most realistic attack path as
"compromised IoT device on the LAN brute-forces the SPA login";
account-lockout + timing-oracle closure + Strict cookies +
no-header CSRF together raise that bar from "~10 min Top-1k
wordlist" to "no programmatic path without 2FA". The supply-chain
hardening (SHA-pinned actions, digest-pinned base images,
`govulncheck`, `npm audit`, Dependabot) closes the highest-impact
long-tail (compromised image-build pipeline) that the review
rated as the worst-case single-event scenario.

Phase 2 (Cosign image signing, audit-log retention + hash-chain,
server-side session store with revocation, externalised
encryption-key requirement) is queued behind this release.

## [0.2.9] - 2026-05-11 — Deploy docs + WebhooksForm a11y fix

Housekeeping pair: the v0.2.8 deploy this morning moved production
from a standalone `docker run` to a container-manager-managed compose
stack, which made the `CLAUDE.md` "Deployment Workflow" section stale.
Plus the lone a11y warning that's been surfacing on every `vite build`
since v0.2.4. Both addressed here. No backend changes.

### Changed

- **`web/src/pages/provision/WebhooksForm.svelte`** — the "enable"
  field label was a `<label class="form-label">` with no `for=`
  attribute and a `<Toggle>` custom component as its sibling, which
  triggered Svelte's `a11y_label_has_associated_control` warning on
  every build since v0.2.4. Changed to `<span class="form-label">`:
  the Toggle component manages its own internal `aria-label` and
  visible on/off text, so the outer wrapper is purely a visual
  heading. Identical rendering, warning gone. Sole instance in the
  codebase (`grep`-verified).
- **`CLAUDE.md`** — "Deployment Workflow" section rewritten to
  document the compose-stack-managed reality:
  - Stack name `shellyadmin` on the container manager, files under
    `<stacks-dir>/shellyadmin/{compose.yaml,.env}`.
  - Stack shape documented (ports `8100:8080` + `8101:8081`, bind
    mount, hardening flags, env vars managed in the container
    manager UI not committed, no `SHELLYADMIN_USER`).
  - Release path: tag push → GHCR build → `pull_image` +
    `start_stack` via the container manager (or its MCP server).
  - Pre-deploy SQLite snapshot recipe preserved.
  - Previous `rsync + ssh docker build + docker run` recipe kept
    as a "Historical (pre-v0.2.8)" subsection with an explicit
    "don't reintroduce as default — port conflicts on 8100/8101"
    caveat.

### Verification

- `vite build` no longer surfaces the WebhooksForm.svelte:97
  warning; bundle size unchanged (309.18 kB raw / 81.10 kB gzip).
- Frontend lint, prettier, vitest (54/54), bundle-size budget all
  green. Backend gates skipped — no Go source touched.

## [0.2.8] - 2026-05-11 — Dep pin refresh (x/net CVE close, Alpine 3.19→3.21)

Periodic dep pin review (originally scheduled ~2026-08-11, pulled
forward today). Audit covered Go direct + indirect deps, npm packages,
Docker base images, CI actions, and the Go toolchain. Two pins moved.

### Changed

- **`go.mod`** — `golang.org/x/net` v0.51.0 → **v0.54.0** and
  `golang.org/x/crypto` v0.48.0 → **v0.51.0**. `go mod tidy` pulled
  transitive bumps of `golang.org/x/sys` (v0.41→v0.44) and
  `golang.org/x/text` (v0.35→v0.37). The `go` directive stayed at
  `1.25.0` — verified with the CLAUDE.md dep-bump-trap check
  (`go list -m … | sort -V -r | head` highest required = `1.25.0`),
  so CI / Dockerfile / `go.mod` remain in sync. No source changes.
- **`docker/Dockerfile:20`** — runtime base `alpine:3.19` → **`alpine:3.21`**.
  Alpine 3.19 reached end-of-community-support in November 2025 and no
  longer receives apk security updates. 3.21 (Dec 2024 release) is
  supported through November 2026 — the minimum jump that gets the
  runtime back inside the support window. Build stage (`golang:1.25-alpine`
  matched to `go.mod`) and frontend build stage (`node:20-alpine`,
  build-only) are unchanged.

### Security

- Closes **GO-2026-4918** (HTTP/2 transport infinite loop on bad
  `SETTINGS_MAX_FRAME_SIZE` frame in `golang.org/x/net`). `govulncheck`
  before the bump reported the vuln in `golang.org/x/net@v0.51.0` as
  "imported but not called" (no reachable call site in ShellyAdmin
  code); after the bump it reports `No vulnerabilities found.`. Defense
  in depth — we did not have a known reachable path, but the fix is now
  in the binary regardless.

### Audited but unchanged

- Other direct Go deps (`gin v1.12.0`, `gin-contrib/sessions v1.1.0`,
  `modelcontextprotocol/go-sdk v1.6.0`, `modernc.org/sqlite v1.34.5`,
  `lumberjack.v2 v2.2.1`) — all on current versions.
- Frontend npm — `npm outdated --json` returned `{}` (the v0.2.0
  major sweep plus the v0.2.7 oxc swap left us caught up).
- CI actions — `actions/checkout@v6`, `actions/setup-go@v6`,
  `actions/setup-node@v6`, `golangci/golangci-lint-action@v9` (v2.6)
  all on current majors.
- Go toolchain — left at 1.25 to match `go.mod`; pre-emptive 1.26
  bump deferred until a forcing function.
- `shellyctl` CLI (pre-v1) — still on backlog, not in this release.

### Verification

- Backend: `go vet ./...`, `golangci-lint run` (v2.6), `go test ./...`
  all green on the bumped deps.
- Vuln scan: `govulncheck ./...` clean post-bump.
- Frontend: `npx prettier --check .`, `npx eslint src/`, `npx vite build`,
  `npm test`, `npm run check:bundle-size` all green; bundle size
  unchanged (no frontend deltas).
- Docker: `docker build` on `alpine:3.21` produces a working image
  carrying the new `shellyctl` binary.

## [0.2.7] - 2026-05-11 — Vite oxc minifier (drop esbuild devDep)

Closes the v0.2.0 tech-debt item: vite 8 made `oxc` the default
minifier and unbundled `esbuild`. v0.2.0 pinned `minify: 'esbuild'` +
the `esbuild` devDep just to keep byte-stable build output across the
v6→v8 rolldown jump. The "esbuild as a separate devDep" was the
cleanup item; this release does it.

### Changed

- **`web/vite.config.ts`** — `minify: 'esbuild'` → `minify: 'oxc'`.
  oxc is rolldown's native transformer (rolldown is vite 8's bundler),
  so the build pipeline is single-tooled now. Comment updated to
  document the swap.
- **`esbuild` removed from `web/package.json` devDependencies**. It was
  added in v0.2.0 only to keep the deprecated `transformWithEsbuild`
  path working.

### Bundle impact

oxc actually shrinks this codebase's output a bit:
- JS raw: 321.95 → **309.18 kB** (-12.77 kB / -4%)
- JS gzip: 89.16 → **81.10 kB** (-8.06 kB / -9%)

Bundle budgets tightened from 328/92 KB → 320/86 KB to leave ~5%
headroom on the new baseline rather than carrying the v0.2.6 ceiling.

### Verification

- Local build with oxc minifier produces a working bundle (vite v8.0.11,
  195 modules transformed, no warnings).
- All 54 vitest cases still pass.
- Frontend lint, prettier, Go test, golangci-lint all green.

## [0.2.6] - 2026-05-11 — Zigbee operations form (write-mostly)

Closes the third "no first-class UI for X" gap. Direct ZCL operations
(`Zigbee.SendCommand` / `Zigbee.ReadAttr` / `Zigbee.WriteAttr`) against
paired Zigbee devices have been routable via the generic `gen2_rpc`
template section since the wave gateway support landed, but operators
had to construct the JSON by hand including the `eui64` (64-bit
device address), endpoint, cluster, and ZCL command/attr arguments.

### Added

- **`web/src/pages/provision/ZigbeeOpsForm.svelte`** — three optional
  operation cards. Each can be toggled on independently; multiple ops
  in the same template build a `gen2_rpc` section with one entry per
  Zigbee.* method (the `gen2_rpc` shape is keyed by method name, so at
  most one of each per template).
  - **Zigbee.SendCommand** — `eui64`, `ep`, `cluster`, `cmd`, optional
    hex `payload`. Empty payload is omitted from the output.
  - **Zigbee.ReadAttr** — `eui64`, `ep`, `cluster`, `attrs` parsed
    from a comma- or whitespace-separated list of integer ids; junk
    values silently dropped.
  - **Zigbee.WriteAttr** — `eui64`, `ep`, `cluster`, `attrs` accepted
    as a raw JSON array of `{id, type, value}` records (the type/value
    pairing is too varied to flatten into a form). Invalid JSON or
    non-array shapes drop the operation.
- State surface in `web/src/pages/provision/state.ts`:
  `createZigbeeOpsState`, `buildZigbeeOps`. Build merges into any
  existing `gen2_rpc` section from `buildTemplate` rather than
  overwriting.
- 9 new vitest cases in
  `web/src/pages/provision/state.test.ts` covering each operation's
  build path, edge cases (empty eui64, empty payload, malformed
  attrs), and the multi-op combination path.

### Constraints

- **Write-mostly form.** The Provision hydrate path doesn't load
  `gen2_rpc` templates back into the form view (existing default-branch
  rejection unchanged). Saving and re-opening a template built with
  this form requires JSON view to inspect — the form is unchecked and
  the `gen2_rpc` section is intact in the JSON.
- **One Zigbee.* method per template.** `gen2_rpc` is method-keyed.
  Operators who need multiple Zigbee.SendCommand calls in one template
  still need JSON view; the form is for the common single-op case.

### Changed

- Bundle-size budget raised 320 → 328 KB raw / 90 → 92 KB gzip for
  the form's ~8 KB raw footprint. New baseline 321.95 KB raw /
  89.16 KB gzip.

## [0.2.5] - 2026-05-11 — Cover (slat-tilt) provisioner form

Closes the second of the "no first-class UI for X provisioner section"
gaps. The `cover` template section has been backend-accepted since the
FW 2.0.0-beta1 wave (per ADR-0008 and the existing
`internal/core/provisioner/provisioner.go:177` handler), but operators
had to drop into JSON view to configure timing, swap_inputs,
power_limit, or the FW 2.0.0-beta1 `slat` sub-object for
venetian-blind tilt.

### Added

- **`web/src/pages/provision/CoverForm.svelte`** — section card with:
  - `id` (component id, defaults to 0 — most blinds are singletons).
  - `name`, `maxtime_open`, `maxtime_close`, `swap_inputs`,
    `power_limit` (each toggleable in classic FieldRow style).
  - `slat` sub-object toggle that reveals the FW 2.0.0-beta1 venetian
    blind tilt controls: `enable`, `open_time`, `close_time`,
    `precise_ctl`, `retain_pos`, `step_pos`.
- State surface in `web/src/pages/provision/state.ts`:
  `createCoverState`, `createCoverSlatState`, `buildCover`,
  `hydrateCover`. The hydrator rejects advanced fields the form
  doesn't surface (`obstruction_detection`, `motor`, `safety_switch`,
  `voltmeter`, `power_meter`, `in_locked`) with a specific error
  pointing at the JSON view.
- 8 new vitest cases in
  `web/src/pages/provision/state.test.ts` covering build and hydrate
  including a full slat round-trip.

### Changed

- Bundle-size budget raised 312 → 320 KB raw / 88 → 90 KB gzip for
  the form's ~11 KB raw / ~3 KB gzip footprint (more fields than
  Webhooks: id + name + 2 maxtimes + swap + power_limit + 6 slat
  sub-fields). New baseline 314.29 KB raw / 87.47 KB gzip.

### Not changed (still JSON-editor-only)

- `obstruction_detection`, `motor`, `safety_switch`, `voltmeter`,
  `power_meter`, `in_locked` — complex nested objects or
  rarely-edited edge-case fields. Templates containing these surface
  a specific hydration error pointing at the JSON view.

## [0.2.4] - 2026-05-11 — Webhooks provisioner form

Closes the "no first-class UI for the `webhooks` provisioner section"
gap. Every other section (`sys`, `mqtt`, `wifi`, `eth`, etc.) has had a
guided form in the Provision page alongside the JSON editor; webhooks
was the conspicuous holdout — operators had to drop into JSON view to
set up HTTP callbacks even for the common wipe-and-replace case.

### Added

- **`web/src/pages/provision/WebhooksForm.svelte`** — a section card
  matching the existing form pattern. Surfaces:
  - `delete_all` toggle (clear every webhook on the device first).
  - Delete by id (comma- or whitespace-separated; junk silently
    dropped).
  - New webhooks: per-row `cid`, `event`, optional `name`, `enable`
    toggle (defaults to On — the form only emits `enable: false`
    when explicitly disabled, matching the Shelly API default), and
    a URLs textarea (one per line, parsed into the `urls` array).
- State surface in `web/src/pages/provision/state.ts`:
  `createWebhooksState`, `buildWebhooks`, `hydrateWebhooks`. The
  hydrator rejects template `webhooks.update` blocks with a clear
  pointer at the JSON editor (per-id updates require per-device
  knowledge of existing webhook ids — out of scope for the form).
- 12 new vitest cases in
  `web/src/pages/provision/state.test.ts` covering both build and
  hydrate paths, including a full `delete_all + delete + create`
  round-trip.

### Changed

- Bundle-size budget raised 300 → 312 KB raw / 86 → 88 KB gzip in
  `web/scripts/check-bundle-size.mjs` for the form's ~6 KB raw /
  ~2 KB gzip footprint. New baseline: 303.56 KB raw / 85.17 KB gzip.

### Not changed (still JSON-editor-only)

- `webhooks.update` — needs per-device id mapping that doesn't fit a
  fleet-wide template form. Hydration of templates that contain an
  `update` block surfaces a specific error pointing at the JSON view.

## [0.2.3] - 2026-05-10 — MCP stdio subcommand + firmware_status paging

ADR-0011 v0.2.x follow-ups, minus per-token scoping (dropped — single
operator, audit-log already attributes per-call request_id).

### Added

- **`shellyctl mcp` stdio subcommand** — exposes the same 21-tool MCP
  surface (read-only + state-changing confirm-gated) on stdin/stdout,
  for "Claude Desktop on the same host" workflows. Wire into Claude
  Desktop's MCP config block. The HTTP MCP listener (port 8081) is
  unchanged for remote-access setups; this is an additive transport.

  Stdio mode trust model:
  - **No transport-level token** — the parent process spawning the
    binary IS the trust boundary. Host filesystem permissions on the
    data dir are the remaining gate.
  - **No background workers** — query session, not a long-running
    server. Avoids races with a parallel container holding the same
    data dir.
  - **Logs to stderr only** — stdout carries JSON-RPC frames.
  - **SQLite WAL mode** handles concurrent readers from the long-running
    server + a stdio subprocess; concurrent writes serialize per WAL
    semantics. Existing job-locking prevents double-starting jobs
    regardless of which transport the request came from.

- **`firmware_status` paging + filtering** — five optional inputs, all
  backward-compatible (zero-valued input reproduces prior behavior):
  `status` (`ok`/`error`/`na`), `has_update` (boolean), `search`
  (substring against MAC or IP, case-insensitive), `limit`, `offset`.
  Output adds `filtered_total` (post-filter count) and `returned`
  (post-page slice length); `running`/`done`/`total` job-level
  metrics unchanged. Lets 200+ device fleets stay under MCP per-tool
  output caps.

### Changed

- **ADR-0011 amended** with a v0.2.3 follow-up section covering the
  stdio trust model, the paging design, and the explicit removal of
  per-token scoping from the v0.2.x roadmap.
- **Roadmap** updates: v0.2.1, v0.2.2, v0.2.3 added to "Recently
  shipped"; MCP follow-ups + Svelte 5 reactivity migration removed
  from "Next"; vite.config oxc minifier swap added as remaining
  v0.2.0 tech-debt.

### Verification

- 8-case unit test in `internal/mcp/tools_test.go` exercises every
  filter combination + paging behavior through the in-memory MCP
  transport.
- Stdio subcommand smoke-tested locally with raw JSON-RPC piped over
  stdin: initialize handshake announces `shellyadmin v0.2.3`,
  tools/list returns the full 21-tool catalog, tools/call list_devices
  returns the empty fleet from a fresh DB, tools/call firmware_status
  with limit=2 returns the new paged shape.

## [0.2.2] - 2026-05-10 — Svelte 5 reactivity migration

Closes the four lint rules deferred during the v0.2.0 frontend dep bump.
The disable list in `web/eslint.config.js` is empty for the first time
since the bump landed.

Three small commits in increasing order of risk:

### Fixed (or made fixable)

- **`svelte/no-useless-mustaches`** — `UserCAForm.svelte:139` placeholder
  was `{'…\n…'}` JS-string mustache; now plain attribute with `&#10;`
  numeric character references that survive HTML attribute parsing as
  newlines.
- **`no-useless-assignment`** — `Provision.svelte:312,319` are false
  positives. The two `autoSelectedCredentialRef = …` writes look dead to
  the intra-block analyser but are read on the NEXT reactive run (the
  `=== autoSelectedCredentialRef` guards above) to decide whether the
  user has overridden the auto-pick. Inline `eslint-disable-next-line`
  with a comment block above explaining why ESLint can't see across
  reactive-block invocations.
- **`svelte/require-each-key`** — 21 `{#each}` blocks across 11 files
  now carry stable keys: `device.mac` for device tables, `link.path`
  for nav, `option.value` for selects, `group.name` for groups,
  `log.id` for log rows, `capability.id`/`action.id` for device-detail
  lists, `column.key` for column visibility, `result.ip` for upload
  results, `s.section` for provision section results, `r.info.ip` for
  provision per-device rows. Two are intentional index keys: ScriptsForm
  (entries' `id` field is bound to a user-editable input, so keying by
  id would confuse Svelte across edits), and Compliance custom_rules
  (rules are user-editable with no stable identity field).
- **`svelte/prefer-svelte-reactivity`** — two top-level reactive `Set`
  fields migrated to `SvelteSet` from `svelte/reactivity`:
  `Groups.svelte` and `Provision.svelte`'s `selected` selection state.
  This drops the Svelte-4-era immutable-reassignment idiom
  (`selected.add(x); selected = new Set(selected)`) — `SvelteSet`
  mutations trigger reactivity directly via Svelte 5's signal system.
  Reassignment retained where the whole set is rebuilt from a different
  iterable (`new SvelteSet(devices.map(...))`); `.clear()` replaces
  `= new Set()`. `UserCAForm`'s `selected: Set<string>` prop is
  type-compatible — `SvelteSet extends Set`. Two `new Set()` sites are
  inline-disabled — local non-reactive helpers inside function bodies
  (Provision.svelte:269 IIFE dedup, Firmware.svelte:374 toggle-all
  union helper).

### Bundle

- +5 KB raw / +1 KB gzip from the `svelte/reactivity` runtime helpers
  (now `297.07 KB` / `83.42 KB`). Still inside the v0.2.0 budget caps
  of `300 KB` / `86 KB`.

## [0.2.1] - 2026-05-10 — Entrypoint args passthrough

One-line bugfix release. The Docker entrypoint script never passed
`docker run` CMD args through to the binary, so the
`docker run --rm <image> shellyctl hash-password <plaintext>` recipe
advertised in README, docs, and CHANGELOGs since v0.0.15 has always
panicked on missing `SHELLYADMIN_PASS_HASH` instead of printing a hash.

Discovered during the v0.2.0 production deploy when the very first
operator action — generating a hash — required an `--entrypoint
/usr/local/bin/shellyctl` workaround.

### Fixed

- `docker/entrypoint.sh` now `exec`s `shellyctl "$@"` so docker-run
  CMD args reach the binary's subcommand dispatcher at
  `cmd/shellyctl/main.go:38`. The no-args path (`docker run <image>`
  with `SHELLYADMIN_PASS_HASH` set on the env) continues to work
  identically.

### Changed (operator-facing)

- The supported `hash-password` invocation drops the leading `shellyctl`
  (which was always wrong since the entrypoint already runs `shellyctl`):

  ```
  # Before (panics on every version v0.0.15 → v0.2.0):
  docker run --rm ghcr.io/buliwyf42/shellyadmin:vX.Y.Z shellyctl hash-password '<plaintext>'

  # After (v0.2.1+):
  docker run --rm ghcr.io/buliwyf42/shellyadmin:vX.Y.Z hash-password '<plaintext>'
  ```

- README.md, docs/SECURITY.md, docs/DEPLOYMENT.md, docker-compose.yml,
  docker/docker-compose.yml, CLAUDE.md, and the v0.2.0 CHANGELOG entry's
  migration recipe all updated to the corrected form.

### Verification

Local docker build of v0.2.1 confirms three behaviors:
- `docker run --rm <image> hash-password 'changeme'` prints a
  `$argon2id$...` PHC string (was: panicked).
- `docker run --rm <image>` with no args and no `_HASH` panics on
  missing `SHELLYADMIN_PASS_HASH` (unchanged baseline).
- The legacy buggy invocation `docker run --rm <image> shellyctl
  hash-password '...'` still panics (Docker passes `shellyctl
  hash-password ...` as CMD; entrypoint passes through; binary sees
  `os.Args[1] == "shellyctl"` not `"hash-password"`). The panic message
  points at the corrected recipe via the doc references.

## [0.2.0] - 2026-05-10 — Plaintext PASS removed + frontend dep major bumps

The first 0.2.x cut. Two bundled chunks the v0.0.15 deprecation window
and the v0.1.14 dep rollback always pointed at: drop the deprecated
`SHELLYADMIN_PASS` plaintext env var and pull the deferred frontend
major-version bumps. Earliest target in the prior roadmap was
2026-07-22; cut early because the project still has a single operator
and the deprecation overlap protects nobody else.

### Breaking

- **Removed** `SHELLYADMIN_PASS` (plaintext admin password) and the
  matching `SHELLYADMIN_PASS_FILE` indirection. `SHELLYADMIN_PASS_HASH`
  (argon2id PHC from `shellyctl hash-password`, optionally via
  `SHELLYADMIN_PASS_HASH_FILE`) is now the only entry point for the
  admin password. Missing `SHELLYADMIN_PASS_HASH` panics at startup
  with a pointer to the `shellyctl hash-password` helper.
  Operator migration is one-time: run
  `docker run --rm ghcr.io/buliwyf42/shellyadmin:v0.2.1 hash-password <plaintext>` (use `:v0.2.1` or later — `:v0.2.0` had an entrypoint args bug, see v0.2.1 entry)
  and replace `SHELLYADMIN_PASS=…` with `SHELLYADMIN_PASS_HASH=<PHC>`.

### Changed

- **Frontend dep majors** (deferred from v0.1.14):
  - TypeScript 5.9 → 6.0
  - Vite 6.4 → 8.0 (rolldown bundler under the hood)
  - `@sveltejs/vite-plugin-svelte` 5 → 7 (paired; vite 8 requires plugin 7)
  - ESLint 9 → 10 + `eslint-plugin-svelte` 2 → 3 + `svelte-eslint-parser` 0.43 → 1
  - `eslint-config-prettier` 9 → 10, new `@eslint/js` ^10
- **Bundle-size budgets** raised 280 → 300 KB raw / 80 → 86 KB gzip in
  `web/scripts/check-bundle-size.mjs` to absorb rolldown's larger output
  versus rollup. New baseline 292 KB raw / 82 KB gzip.
- **`web/vite.config.ts`** comment updated; `minify: 'esbuild'` is now
  explicit (vite 8 made oxc the default and unbundled esbuild). New
  devDep `esbuild ^0.27` keeps build output byte-stable across the
  rollup → rolldown switch.

### Fixed

- **`Provision.svelte:237`** — real reactivity bug surfaced by
  `svelte/no-immutable-reactive-statements` (newly default in
  eslint-plugin-svelte 3). The `$: precheckTemplate = templateForPrecheck()`
  statement only re-ran when the function reference itself changed, not
  on the 17 underlying state vars `buildTemplate()` reads. Replaced with
  an explicit comma-operator dep list so Svelte's compile-time tracker
  sees every dep.

### Deferred (tracked as v0.2.x follow-ups)

Five new ESLint 10 / eslint-plugin-svelte 3 default rules were disabled
in `web/eslint.config.js` rather than mass-edited under auto mode. Each
points at real but substantial work tracked in `docs/roadmap.md`'s
"Svelte 5 reactivity migration" entry:

- `svelte/require-each-key` — ~16 `{#each}` blocks need stable keys
- `svelte/prefer-svelte-reactivity` — 4 sites use `new Set` where
  Svelte 5's `SvelteSet` would track mutations natively (changes the
  reactivity pattern from immutable-reassignment to mutation)
- `svelte/no-useless-mustaches` — `UserCAForm.svelte:139`
- `no-useless-assignment` — `Provision.svelte:291,298` (control-flow
  analysis across conditional branches in `autoSelectedCredentialRef`
  writes)

### Migration checklist

Before pulling v0.2.0:

1. If you set `SHELLYADMIN_PASS` today, generate a hash:
   `docker run --rm ghcr.io/buliwyf42/shellyadmin:v0.2.1 hash-password <plaintext>` (use `:v0.2.1` or later — `:v0.2.0` had an entrypoint args bug, see v0.2.1 entry)
2. Replace `SHELLYADMIN_PASS=…` with `SHELLYADMIN_PASS_HASH=<PHC>` (or
   the `_FILE` indirection) on the container's environment.
3. Pull and recreate. Missing `_HASH` panics at startup, so misconfigs
   fail loudly rather than silently.

## [0.1.23] - 2026-05-10 — RefreshDevice name lookup

Tiny patch caught by the v0.1.22 live demo. When the LLM ran the
state-changing flow against a device by name (the natural way for
an operator to talk about a device), `execute_device_action` worked
but `refresh_device` errored "device not found" — same target,
different lookup paths.

`services.RefreshDevice` had its own MAC/IP-only lookup loop
parallel to the one in `GetDeviceDetail` that we already extended
with `Name` matching in v0.1.19. Single-line fix at
[internal/services/app.go:160](internal/services/app.go) plus a
test mirroring `TestGetDeviceDetailResolvesByMACOrIPOrName`.

### Fixed
- `services.RefreshDevice` now resolves targets by **MAC, IP, or
  Name**, matching the contract MCP tools and the HTTP API
  advertise. No other call sites had the same bug — confirmed via
  `grep -nE 'devices\[i\]\.MAC == target'` over `internal/`.

### Tests
- New `TestRefreshDeviceResolvesByMACOrIPOrName` in
  `internal/services/app_jobs_test.go`. Uses a 0.5-second refresh
  timeout so the probe (against an unreachable test IP) fails fast
  without slowing the suite — the assertion is on the lookup loop,
  not the probe.

### Migration notes
None.

## [0.1.22] - 2026-05-09 — State-changing MCP tools, confirm-gated

Lifts the v0.1.19–v0.1.21 read-only restriction on the MCP surface.
Eight new tools let LLM-driven agents trigger refreshes, scans,
firmware checks/installs, per-device actions (reboot, factory reset,
ota_revert, switch toggles, cover open/close, etc.) and bulk
configuration changes. Every state-changing tool requires an
explicit `confirm: true` parameter to execute; without it, returns
a structured **preview** describing what would happen so the LLM
can summarize it for the operator and obtain approval before
proceeding. The tool description spells out the policy verbatim.

ShellyAdmin-config writes (settings, credentials, templates,
provisioning, log clearing) remain hard-excluded — those still go
through the SPA where they belong.

### Added — 8 state-changing tools
- `refresh_device(target, confirm)` (risk: low) — re-probe one device.
- `refresh_all_devices(confirm)` (risk: low) — fleet-wide refresh job.
- `start_scan(confirm)` (risk: low) — discovery scan over configured
  subnets. Discovered devices land in `scan_status.pending`.
- `confirm_scan(macs, confirm)` (risk: medium) — register devices
  found by the most recent scan. `macs=[]` means register everything.
- `firmware_check(confirm)` (risk: low) — query stable+beta firmware
  versions for every known device.
- `firmware_install(macs, stage, confirm)` (risk: high) — trigger
  `Shelly.Update` against the named devices on the chosen channel.
  Validates `stage` ∈ {"stable", "beta"}.
- `execute_device_action(target, action, stage?, confirm)` (risk:
  varies, surfaced in preview) — run any per-device action from
  `list_device_actions`. Preview includes the catalog risk level
  ("low" / "medium" / "high") so the LLM knows how loud to be when
  asking for approval.
- `bulk_action(action, macs, value?, confirm)` (risk: high) — apply
  fleet-wide setting changes (set_timezone, set_sntp_server,
  set_mqtt_server, set_auto_update, etc.). The preview reuses
  `services.PreviewBulkAction` so the LLM sees per-target
  eligibility (offline, locked, missing capability) before asking.

### Confirm flow
Every tool input has a `Confirm bool` field, default false. With
`confirm` omitted or false, the tool returns a typed preview output
(`SimpleActionResult.Preview = true`, plus tool-specific fields like
`device_count`, `target_count`, `risk`, or per-target eligibility)
and does not call into the AppService action method. With
`confirm: true`, the tool delegates to the AppService method as
usual. Each invocation writes to `audit_log` with a
`mode=preview` or `mode=confirmed` tag so an operator grepping
`/api/logs` can pair preview/execute calls by `request_id` and see
exactly what ran.

Risk-aware audit: `actionTool` wraps the call context with
`services.WithRisk(ctx, "low|medium|high")` before invoking the
underlying method, so audit rows carry `risk_level` (the v0.1.10
column) for filterability. `execute_device_action` is tagged "high"
at the audit boundary; the preview separately surfaces the
catalog-defined per-action risk so the LLM can adapt its prompt.

### Tests
- `internal/mcp/tools_actions_test.go` — 7 cases exercising the
  preview gate end-to-end through the MCP transport:
  `TestRefreshAllDevicesPreviewVsConfirm` (the omitted-confirm and
  explicit-confirm:false paths both preview),
  `TestStartScanPreview`, `TestFirmwareInstallRequiresConfirmAndStage`
  (stage validation kicks in even in preview),
  `TestExecuteDeviceActionPreviewSurfacesRisk`,
  `TestExecuteDeviceActionRejectsUnknownAction`,
  `TestBulkActionPreviewListsTargets`,
  `TestActionAuditLogsPreviewVsConfirmed` (audit rows carry
  mode=preview when no confirm).
- `internal/mcp/tools_test.go` — `connectInMemory` now wires the
  service's logFn to `database.AddLog` so audit-aware tests can
  inspect what each MCP call produced.

### Migration notes
None. The new tools are additive — clients that don't know about
them continue to use the read-only surface unchanged. Existing
`tools/list` callers will see the surface grow from 13 → 21 tools.
Token resolution, transport, encryption, and the v0.1.21 live
toggle behavior are unchanged.

## [0.1.21] - 2026-05-09 — Live MCP toggle (no restart required)

Lifts the v0.1.20 restart-required posture for the MCP toggle.
Saving settings with `mcp_enabled` true / false / a rotated token now
**applies immediately** — the listener starts, stops, or rotates its
token in-process without dropping the rest of the container. Token
rotations no longer kick MCP clients (Claude Desktop, etc.) any
harder than the rotation itself requires.

The change came with one architectural fix: `api.NewHandler`
previously constructed its own `services.NewAppService(...)`
internally, parallel to the one in `main.go`. HTTP handlers and the
boot-time service couldn't see each other's in-memory state — which
is why surfacing live MCP status would have been impossible without
this fix. `api.Config.Service` is the new entry point; when set,
NewHandler reuses it. `main.go` always sets it. Background workers,
audit-log routing, and the MCP controller now live on one
process-wide service.

### Added
- `internal/services/app_mcp.go`: new `MCPController` struct holding
  the live `*http.Server` and a mutex serializing start / stop /
  rotate transitions. `SetMCPParams` installs runtime params
  (database, builder, env-token, bind, port, version);
  `StartMCPFromConfig` does boot-time env-or-settings resolution;
  `ReconcileMCPFromSettings` is called by `SaveSettings` after a
  successful persist; `stopMCP` is invoked from `Stop(ctx)` so
  graceful shutdown drains the listener before background workers.
- `MCPBuilder` is a function-typed seam on the controller. Production
  code injects `mcp.Build` from `main.go` — passing it through (vs.
  importing it in `internal/services`) avoids a services↔mcp import
  cycle since the mcp package itself imports services for its tools.
- `models.AppSettings.MCPRunning` (read-only, omitempty), populated
  by the API GET handler from `service.MCPRunning()`. Drives a new
  green `Running` / grey `Stopped` badge on the Settings page MCP
  card.
- `api.Config.Service *services.AppService` — when non-nil,
  `NewHandler` reuses the externally-supplied AppService instead of
  constructing its own. Required for the controller-sharing fix
  above.
- 5 new tests in `internal/services/app_mcp_test.go` covering the
  lifecycle: `TestSaveSettingsStartsAndStopsMCPLive`,
  `TestSaveSettingsRotatesMCPTokenLive`,
  `TestSaveSettingsIsNoOpWhenEnvLocked`,
  `TestStartMCPFromConfigPrefersEnvOverSettings`,
  `TestStartMCPFromConfigUsesSettingsWhenEnvUnset`. Each uses a
  test-only `MCPBuilder` that returns `*http.Server`s bound to
  `httptest` listeners so port collisions don't break parallel test
  runs.

### Changed
- `cmd/shellyctl/main.go`: AppService is now constructed before
  `api.NewRouter` and passed into `Config.Service`. The MCP startup
  block delegates to `service.SetMCPParams` + `service.StartMCPFromConfig`
  instead of building the listener inline; `service.Stop(ctx)` now
  shuts the listener down (the previous explicit `mcpServer.Shutdown(ctx)`
  call was removed).
- `internal/api/handler.go`: `GetSettings` populates `MCPManagedByEnv`
  and `MCPRunning` via `h.service.MCPManagedByEnv()` /
  `h.service.MCPRunning()` instead of reading `os.Getenv` directly.
- `internal/services/app.go`: `SaveSettings` calls
  `s.ReconcileMCPFromSettings()` after the successful DB write.
  `Stop(ctx)` calls `s.stopMCP(ctx)` first so the externally-visible
  surface drops new requests before background workers drain.
- `web/src/pages/Settings.svelte`: MCP card hint text updated to
  "Saves apply immediately"; "Enable MCP server on next restart"
  → "Enable MCP server"; new live status badge in the card header;
  Save handler re-fetches `/api/settings` after success so the
  redacted-token placeholder and Running/Stopped badge update without
  a manual reload.

### Live verification (44-device fleet)
- Boot via env var → MCP up; toggle off in Settings UI **without
  env**: connection refused on `:8101` (listener stopped, port unbound).
- Toggle on with new token: new token returns 200, old token returns
  401 — listener restarted with the rotated token.
- Token rotation while listener already running: old listener's
  context is cancelled (5-sec graceful), new listener starts; total
  outage <100 ms.
- API GET reports `mcp_running: true` while listener is live, omitted
  (false) when stopped — so the SPA badge tracks reality.

### Migration notes
None for the documented config surface — `SHELLYADMIN_MCP_TOKEN` env
var still wins, and the persisted settings shape is unchanged from
v0.1.20. Code consumers of `internal/api`: `Config.Service` is
optional but strongly recommended; tests that don't set it still
work (`NewHandler` falls back to constructing its own service for
that case). DB schema is unchanged.

## [0.1.20] - 2026-05-09 — Settings UI for MCP + page reorganization

Brings the v0.1.19 MCP server out of env-only territory: operators can
now enable, disable, and rotate the MCP token from the Settings page,
without touching the container's `docker run` line. The env var
(`SHELLYADMIN_MCP_TOKEN`) still takes precedence, preserving the
operator-override path for headless / CI / Compose-managed deploys —
when it's set, the UI fields render read-only with a "managed by
environment variable" notice. The Settings page itself was
reorganized as part of the same change: 3 mixed cards became 5
focused cards (Discovery & Refresh, Firmware, MCP, Display, Backup).

### Added
- `models.AppSettings` gains `MCPEnabled bool` and `MCPToken string`,
  plus a read-only `MCPManagedByEnv bool` populated by the API GET
  handler. Persisted token is encrypted at rest via
  `internal/core/secretbox` (NaCl secretbox; same envelope used for
  credential passwords and HA1 hashes).
- `services.SaveSettings` seals the plaintext token before writing;
  `services.GetSettings` opens it for internal callers. The API GET
  handler in `internal/api/handler.go` re-redacts the token to a
  `<set>` placeholder (exposed as `services.MCPTokenRedacted`) before
  the response leaves the process — plaintext never crosses the wire
  to the SPA. `<set>` round-trips as "preserve the existing stored
  token" on save, so the SPA can re-submit settings without exposing
  or accidentally clobbering the secret.
- `cmd/shellyctl/main.go` MCP-startup block now consults
  `services.GetSettings()` when `SHELLYADMIN_MCP_TOKEN` is unset, and
  enables the listener with the persisted token if
  `MCPEnabled && MCPToken != ""`. Startup logs which path activated:
  `MCP enabled via settings (env var not set)` for the new path,
  `MCP server starting addr=…` for both. When neither source supplies
  a token, the existing `MCP disabled (no token in env or settings)`
  log line fires.
- `ValidateSettings` rejects `MCPEnabled=true` with a token shorter
  than 16 characters.
- New MCP card on the Settings page (`web/src/pages/Settings.svelte`):
  Enable toggle, password-style token input with **Show / Hide /
  Generate / Copy / Clear** buttons. Generate uses
  `crypto.getRandomValues` to produce 64 hex chars (same length as
  `openssl rand -hex 32`). Per-state hint text guides the operator
  through "no token" → "token in form, not saved" → "token configured
  (`<set>`)". When `mcp_managed_by_env` comes back true, all controls
  on the card are disabled and the override notice replaces the
  hint text. UI cleanup: the Settings page reorganized from 3 cards
  (Discovery+Refresh+Firmware mixed, UI Preferences, Backup) into 5
  (Discovery & Refresh, Firmware, MCP, Display, Backup) with `h-100`
  on each card so the column heights line up. UI Preferences renamed
  to **Display** for accuracy.
- ADR-0011 amended with a "v0.1.20 follow-up — Settings-driven
  configuration" section documenting the precedence rule, encryption
  approach, redaction boundary, validation, and the explicit
  restart-required-vs-live-toggle decision.

### Tests
- `internal/services/app_test.go`:
  `TestSaveSettingsEncryptsMCPTokenAndRoundTripsPlaintext` confirms
  the persisted form differs from plaintext but `GetSettings` returns
  the original;
  `TestSaveSettingsPreservesTokenWhenSentRedactedPlaceholder` confirms
  the SPA can round-trip settings without clobbering the stored
  token; `TestValidateSettingsRejectsShortMCPToken` and
  `TestValidateSettingsAllowsEmptyTokenWhenMCPDisabled` cover the new
  validation rule.

### Live verification (44-device fleet)
- API GET with env set returns `mcp_managed_by_env: true`,
  `mcp_token: "<set>"`.
- API POST with a fresh plaintext token persists encrypted; restart
  without the env var brings up MCP using the persisted token
  (`MCP enabled via settings (env var not set)`); both header and
  URL-path auth succeed with the new token. The previously valid
  env-var token is correctly rejected once the env is unset.
- Restoring the env var brings precedence back to the env path
  immediately on next restart.

### Migration notes
None. Existing deployments using `SHELLYADMIN_MCP_TOKEN` continue to
work unchanged — the env var still wins. Operators who want to migrate
to settings-driven config can: configure the token in the Settings UI
and verify Save shows `<set>`, drop the env var from `docker run` /
compose, restart. The persisted token then drives MCP startup. The
DB schema is unchanged (settings are stored as JSON), so downgrades
to v0.1.19 silently ignore the new fields.

## [0.1.19] - 2026-05-09 — Optional read-only MCP server

First feature-surface expansion since v0.1.12. Adds an opt-in,
read-only Model Context Protocol server embedded in the existing
binary so LLM-driven agents (Claude Desktop, Claude Code, custom MCP
clients) can introspect the fleet — without exposing any
state-changing operation. Listener is off unless `SHELLYADMIN_MCP_TOKEN`
is set; gated by static bearer-token auth. Design rationale in
[ADR-0011](docs/adr/0011-mcp-read-only-server.md).

### Added
- New package `internal/mcp/` (server, auth, redact, tools, tests).
  Streamable HTTP transport via the official
  `github.com/modelcontextprotocol/go-sdk` v1.6.0; typed-generic
  `mcp.AddTool[In, Out]` so each tool's JSON schema is generated from
  its Go input struct.
- 13 read-only tools, all thin adapters over `services.AppService`:
  - **Devices**: `list_devices` (with `search` / `gen` / `limit`
    filters), `get_device`, `list_device_actions`, `export_device`.
  - **Job status**: `scan_status`, `firmware_status`,
    `firmware_install_status`.
  - **Configuration**: `list_templates`, `get_template`,
    `list_credentials` (redacted — never returns plaintext password
    or HA1), `get_settings`.
  - **Audit & compliance**: `get_logs` (with `level` / `search` /
    `risk` / `limit`), `compliance_summary`.
- `internal/mcp/redact.go` + `redact_test.go` enforce the
  no-plaintext-secrets rule in code, not docs. The credential output
  type omits Password and HA1 fields entirely so even an accidental
  marshal cannot leak them.
- Auth middleware accepts the static token via either the standard
  `Authorization: Bearer <token>` header or a URL whose first path
  segment IS the token (`http://host:8081/<token>/`, the same shape
  Home Assistant's MCP integration uses) — convenient for clients like
  `mcp-remote` where header args are awkward. Both checks run through
  `subtle.ConstantTimeCompare`; the matched path prefix is stripped
  before reaching the SDK handler. HTTP listener returns plain `401
  unauthorized` for missing / wrong tokens.
- Request-ID middleware honours an inbound `X-Request-ID` header
  (sanitized to `[A-Za-z0-9_-]{1,64}`) or generates a fresh 16-hex-char
  id, then propagates via `middleware.WithRequestID` so every tool
  call's audit row carries it. Echoes the value back on the response
  for client-side correlation.
- New env vars (parsed in `cmd/shellyctl/main.go`):
  - `SHELLYADMIN_MCP_TOKEN` (required to enable; `_FILE` indirection
    supported via `services.DecodeSecretValue`).
  - `SHELLYADMIN_MCP_PORT` (default `8081`).
  - `SHELLYADMIN_MCP_BIND` (default `0.0.0.0`; set to `127.0.0.1` for
    loopback-only).
- ADR-0011 documents the design (read-only-first scope, transport
  choice, secret-hygiene boundary, alignment with the planned
  `shellyctl` CLI).

### Hard exclusions in v1
Refresh, scan trigger, scan confirm, firmware check, firmware update,
firmware install, provision, upload-CA, save/delete templates,
save/delete credentials, save settings, clear logs, run bulk action,
set auto-update. State-changing tools are deferred — they need a
confirmation/audit-trail design that the read-only baseline does not
provide. See ADR-0011 "Hard exclusions" and the roadmap "Next (pre-v1)"
entry for v0.2.x follow-ups.

### Audit logging
Every tool invocation logs at `info` (or `warn` on tool error) through
`service.LogCtx(ctx, ...)`. Entries appear in `/api/logs` and the
SPA's Logs page filterable by request_id, prefixed with `mcp `.

### Container
- `docker/Dockerfile` adds `EXPOSE 8081`. Existing
  `:8080/health` healthcheck unchanged.
- `docker-compose.yml` adds a commented-out `8081:8081` port mapping
  and a commented-out `SHELLYADMIN_MCP_TOKEN` env line so operators
  see the opt-in path. Pinned image tag bumped from `v0.0.5` (very
  stale) to `v0.1.19`.

### Dependency bump check
Adding `github.com/modelcontextprotocol/go-sdk@v1.6.0` (and its
transitives — `google/jsonschema-go`, `yosida95/uritemplate`,
`golang/oauth2`, `segmentio/encoding`, `golang-jwt/jwt`) does **not**
raise the `go.mod` directive. Dep-bump-trap check (per CLAUDE.md)
keeps `go 1.25.0` at the top.

### Fixed (post-deploy refinements, same day)
- `scan_status.pending` now returns a slim per-device summary
  (`{mac, ip, name, model, gen, app}`) instead of the full
  `models.Device` shape with its ~150-entry `supported_methods` list.
  On a 44-device fleet the response shrank from ~63 KB → ~7.5 KB,
  fitting under MCP client per-tool output caps. The SPA's scan
  workflow keeps the full shape — only the MCP adapter slims it.
- `services.GetDeviceDetail` now resolves targets by name in addition
  to MAC and IP. The MCP tool descriptions for `get_device`,
  `list_device_actions`, and `export_device` advertised name
  resolution; the underlying lookup did not, so name-based calls
  errored with `device not found`. Fix is at the service layer so all
  four callers benefit.

### Migration notes
None. New env var is opt-in; when unset, MCP is off and the listener
does not bind. No DB migration. No public-signature changes to the
existing HTTP API. Both auth shapes use the same `SHELLYADMIN_MCP_TOKEN`,
so existing bearer-header configs continue to work unchanged.

## [0.1.18] - 2026-05-08 — Setters round-out + provisioning integration smoke

Step 3 (final) of the M3 testability foundation. Closes the coverage gap
on `internal/core/setters` (was 32.1%) and adds the multi-section
end-to-end smoke called for in the M3 plan.

### Added
- `internal/core/setters/setters_more_test.go` (~6 test groups, ~13
  cases including subtests):
  - `SetLocation` (lat/lon under `params.config.location`),
  - `SetSNTPServer` (different nesting under `params.config.sntp`),
  - `SetCoverTilt` percent-clamping table (in-range / negative / over /
    boundary),
  - method-not-found path (Shelly 404 + JSON-RPC -32601 both make the
    setter return false — the bulk-action UI's silent-skip contract),
  - `CoverOpen` happy-path detail string (representative of the
    `(bool, string)` returner family),
  - `BLEPair` with all three branches: happy path, 404 →
    `supported=false`, and 401-with-Digest → `supported=true` but
    `ok=false` (the supported-but-unreachable distinction the
    per-device action layer relies on).
- `internal/core/provisioner/integration_smoke_test.go` —
  `TestProvisionDevice_MultiSectionSmoke`. One template carrying sys +
  mqtt + wifi + auth drives a single `ProvisionDevice` call; verifies
  every section ends `Status="ok"`, the expected RPCs were issued
  exactly once each, the `{device_name}` token was hydrated from the
  preflight, and the `Shelly.SetAuth` HA1 is the correct
  `SHA-256("admin:serial:pass")` (the highest-risk computation in the
  provisioner — a wrong hash silently locks operators out).

### Coverage
- `internal/core/setters`: 32.1% → **56.4%** (+24.3 pp).
- `internal/core/provisioner`: → **61.7%** (already had cases; the
  cross-section smoke is new value).

### Migration notes
None. Test-only release. No public-signature changes, no DB migration,
no env-var change.

## [0.1.17] - 2026-05-08 — Firmware + scanner unit tests on the OnClient seams

Step 2 of the M3 testability foundation. Uses the Clock + OnClient seams
landed in v0.1.15 to back the previously-untested `internal/core/firmware`
package and the failure-handling branches of `internal/core/scanner` with
fast (sub-second), deterministic, network-free unit tests.

### Added
- `internal/core/firmware/helpers_test.go` — shared `fakeShelly`
  fixture: a httptest server with a per-method handler map and a call
  recorder. Unregistered methods return Shelly's non-standard 404 RPC
  error so `IsMethodNotFound` paths can be tested realistically.
- `internal/core/firmware/firmware_test.go` (10 cases) covering
  `CheckOneOnClient` (happy path, no-update case, GetDeviceInfo
  failure, CheckForUpdate failure), `TriggerUpdateOnClient` (request
  shape + 401-with-Digest + 429 lockout), `GetDeviceFirmwareOnClient`
  (ver-vs-fw fallback table + RPC error), and the gen<2 short-circuit
  on `CheckOneWithOptions`.
- `internal/core/firmware/methods_test.go` (3 cases) covering
  `ListSupportedMethodsOnClient` (sort + filter, the Shelly 404 vs
  JSON-RPC -32601 quirk that CLAUDE.md flags), and the gen<2
  short-circuit on the `…WithOptions` wrapper.
- `internal/core/firmware/autoupdate_test.go` (9 cases) covering the
  Schedule.\*-based auto-update read/write path called out in
  ADR-0009: `ReadAutoUpdateOnClient` (off/stable/beta/disabled-job/
  user-created-job/case-insensitive-method) and
  `SetAutoUpdateOnClient` (off-deletes-only-shelly_service-jobs,
  stable-creates-with-expected-shape, invalid-mode-rejected,
  empty-mode-canonicalises-to-off).
- `internal/core/scanner/probe_clock_test.go` (5 cases) — pins the
  `LastSeen`-via-FakeClock and `AuthLockedUntil`-via-FakeClock+60s
  contracts, the scan-path-must-return-nil regression class
  (v0.0.16 / v0.1.1 / v0.1.2 fixes around UniFi UDM and friends),
  the refresh-path auth-required partial Device, and the nil-clock
  fallback to `clock.Real()`.
- `firmware.ReadAutoUpdateOnClient` — pre-built-client variant of
  `ReadAutoUpdate`, mirroring the existing `SetAutoUpdateOnClient`
  precedent. The `…WithOptions` wrapper retains its signature.

### Coverage
- `internal/core/firmware`: 0% → **71.1%** (target: ≥60% on JSON-RPC
  translation paths).
- `internal/core/scanner`: 21% → **39.2%** (CIDR / mDNS / ScanSubnets
  concurrency intentionally out of scope; the OnClient JSON-RPC paths
  are well covered).

### Migration notes
None. Test-only release plus one additive helper (`ReadAutoUpdateOnClient`).
No DB migration, no env-var change, no public-signature break.

## [0.1.16] - 2026-05-08 — Platform refresh: Go 1.25 + re-take v0.1.14 deps

Toolchain bump. The Go floor moves from 1.24 to 1.25 across the
build pipeline, which un-blocks the dep upgrades that v0.1.14 attempted
and v0.1.15 had to roll back (gin v1.12 pulled `quic-go` for HTTP/3,
which requires Go 1.25.0). Operator-facing surface is unchanged — the
container is still a static Linux/amd64 binary, image size unchanged
in this build.

### Changed
- **Go 1.24 → 1.25** across the build pipeline:
  - `.github/workflows/test.yml` — both Go-version pins.
  - `docker/Dockerfile` — `golang:1.24-alpine` → `golang:1.25-alpine`
    (backend stage).
  - `go.mod` directive `go 1.24.0` → `go 1.25.0`.
- **Re-took the v0.1.14 dep upgrades** that were rolled back in v0.1.15
  for Go-1.24 compat: `gin-gonic/gin` v1.10.1 → v1.12.0,
  `gin-contrib/sessions` v1.0.4 → v1.1.0, `golang.org/x/net` v0.50.0 →
  v0.51.0, `golang.org/x/text` v0.34.0 → v0.35.0, `golang.org/x/sync`
  v0.19.0 → v0.20.0. Indirect deps follow accordingly (notably
  `quic-go/quic-go` v0.59.0 + `quic-go/qpack` v0.6.0 are pulled in by
  gin v1.12's HTTP/3 support).

### Migration notes
No DB migration. No env-var changes. Operators on v0.1.13 or v0.1.15
upgrade with the usual `docker pull ghcr.io/buliwyf42/shellyadmin:v0.1.16`
+ recreate.

If you build the image yourself (rather than pulling from GHCR), make
sure your local Docker can pull `golang:1.25-alpine` — older Docker
hosts that haven't refreshed their image cache may need a `docker
image rm golang:1.24-alpine` and a fresh `docker build`.

## [0.1.15] - 2026-05-08 — Testability seams + v0.1.14 CI rollback

**Operator-impacting:** v0.1.14's GHCR image never published — its dep
bumps (gin v1.10.1 → v1.12.0) pulled in `quic-go/quic-go` for HTTP/3,
which forced `go.mod` to `go 1.25.0`, but CI runs Go 1.24. Both the
Test and Publish-Image workflows for v0.1.14 failed with
`go.mod requires go >= 1.25.0 (running go 1.24.13)`. **There is no
`ghcr.io/buliwyf42/shellyadmin:v0.1.14` image; upgrade directly from
v0.1.13 to v0.1.15.** The v0.1.14 tag is left in place as a historical
marker but should not be deployed.

This release combines two concerns:

1. **CI fix** — partial rollback of v0.1.14's dep bumps to restore Go
   1.24 compatibility.
2. **M3a (testability seams)** — step 1 of the M3 testability
   foundation from the post-v0.1.12 plan. Pure structural refactor — no
   behaviour change for any production caller. The goal is to make
   `internal/core/{scanner,firmware,setters}` exercisable by
   deterministic httptest-backed unit tests in v0.1.16 / v0.1.17
   without needing to mock the time package globally or stand up real
   network I/O.

### Fixed
- **go.mod restored to `go 1.24.0`.** Rolled back: `gin-gonic/gin`
  v1.12.0 → v1.10.1 (drops the quic-go/HTTP/3 transitive dep);
  `gin-contrib/sessions` v1.1.0 → v1.0.4 (the v1.1.0 release requires
  gin v1.12); `golang.org/x/net` v0.51.0 → v0.50.0; `golang.org/x/text`
  v0.35.0 → v0.34.0; `golang.org/x/sync` v0.20.0 → v0.19.0. The
  `golang.org/x/crypto` v0.45.0 → v0.48.0 bump is preserved (v0.48.0
  still targets `go 1.24.0`). The plaintext-deprecation-warning text
  changes from v0.1.14 are also preserved — this rollback is dep-only.

### Added
- New `internal/core/clock` package — minimal `Clock` interface
  (`Now() time.Time`), `Real()` factory, and `Fake` with `Advance(d)`
  for tests. Two-test coverage: real-clock advances; fake is
  deterministic until `Advance`.
- `firmware.CheckOneOnClient`, `firmware.TriggerUpdateOnClient`,
  `firmware.GetDeviceFirmwareOnClient` — pre-built-client variants
  matching the existing `ListSupportedMethodsOnClient` pattern from
  `firmware/methods.go`. Existing `…WithOptions` callers keep working;
  they now build a client internally and delegate.
- `scanner.ProbeDeviceOnClient` — same shape; takes a pre-built
  shellyclient and an explicit Clock. `ProbeDeviceWithOptions` is a
  thin wrapper.
- `setters.NewWithClient` — wraps a pre-built shellyclient without
  going through Options. Setters has no time-dependent code so no
  Clock plumbing was added there.

### Changed
- `scanner.ProbeOptions` and `firmware.Options` gain an optional
  `Clock clock.Clock` field. Production callers leave it nil; the
  package internally falls back to `clock.Real()`. Surfaces a single
  injection point for tests without changing existing call sites.
- Bare `time.Now()` calls at `scanner.go:144` (LastSeen),
  `scanner.go:185` (AuthLockedUntil), and `firmware.go:78` (CheckedAt)
  now route through the Clock seam.

### Migration notes
None. No DB migration, no env-var change, no public-signature break for
existing callers. All previously-exported `…WithOptions` functions
retain their signatures and behaviour.

## [0.1.14] - 2026-05-08 — Security hygiene: dep pins + plaintext deprecation countdown

Two related cleanups under one release.

### Changed
- **Plaintext password deprecation warning sharpened** with a concrete
  removal target. The startup `slog.Warn` and `docs/SECURITY.md` now
  state that `SHELLYADMIN_PASS` plaintext support is scheduled for
  removal in **v0.2.0, no earlier than 2026-07-22** — the 3-month
  overlap window from the v0.0.15 (2026-04-22) deprecation. Operators
  on plaintext: run `shellyctl hash-password <plaintext>` and switch to
  `SHELLYADMIN_PASS_HASH` (or `_FILE`) before that date. Removal itself
  is **not** in this release.
- **Conservative dependency bumps** (patch + minor only; majors deferred
  to dedicated releases):
  - Go: `gin-gonic/gin` v1.10.1 → v1.12.0; `gin-contrib/sessions`
    v1.0.2 → v1.1.0. `go mod tidy` rolled forward indirect deps —
    notably `gorilla/sessions` v1.2.2 → v1.4.0, `golang.org/x/crypto`
    v0.45.0 → v0.48.0, `golang.org/x/net` v0.47.0 → v0.51.0, and
    several supporting libraries — none of which are direct API
    surface.
  - npm: in-range patches for `@typescript-eslint/*`, `typescript-
    eslint`, `@vitest/ui`, `jsdom`, `svelte`, `vitest`. `npm audit`
    reports 0 vulnerabilities. Major-version updates for `eslint`,
    `vite`, `typescript`, `eslint-plugin-svelte`, `svelte-eslint-
    parser`, `eslint-config-prettier`, and `@sveltejs/vite-plugin-
    svelte` are deferred per the conservative-bump policy.

### Migration notes
No DB migration. No env-var changes. Operators on plaintext should see
the sharpened deprecation warning on next startup; act on it before
2026-07-22 to avoid a hard panic in v0.2.0.

## [0.1.13] - 2026-05-08 — Configurable firmware-install poll cadence

First item from the post-v0.1.12 field-test pause: a small operator knob.
The firmware install_job's per-device version-recheck cadence (the loop
that watches a device come back up after `Shelly.Update`) was previously
hardcoded at 5 s. It is now an AppSetting, bounded `[1, 60]`, default 5.

### Added
- `AppSettings.FirmwareInstallPollInterval` (seconds; default 5; bounds
  `[1, 60]` enforced in `Normalize`). Surfaced on the Settings page next
  to the existing Install timeout field. Lower it for snappier feedback
  on a small fleet, raise it to be gentler on slow devices.
- `firmwareInstallPollIntervalFromSettings` helper mirrors the existing
  timeout helper; unit tests cover both the helper and the Normalize
  bounds.

### Changed
- The firmware install_job's `runFirmwareInstallJob` / `installOne`
  signatures gain a `pollInterval time.Duration` parameter, threaded
  from `StartFirmwareInstall` after a `db.GetSettings` read. The const
  `firmwareInstallPollInterval` is renamed
  `defaultFirmwareInstallPollInterval` and now serves only as the
  fallback when the settings row pre-dates the field.

### Migration notes
No DB migration. Existing settings rows will read `0` for the new field
on first load; both Normalize and the helper treat that as "use default
5", so no operator action is required. The field round-trips through
the JSON API and the backup/import flow without further changes.

## [0.1.12] - 2026-05-07 — Logs risk filter + batch / fw_id on the detail page

Three small operator-facing improvements layered on the v0.1.10 audit
risk_level + v0.1.11 friendly-label work. Includes migration
`024_device_batch_fwid.sql`.

### Added
- **Logs page risk filter.** New dropdown next to the Level filter:
  Off / Low / Medium / High. Filters the audit_log query server-side
  via the new `?risk=` URL param and the `db.GetLogsFiltered` /
  `db.GetLogsForExportFiltered` write surface; CSV/NDJSON export
  honours the filter the same way Level + Search already do.
- **Batch + Firmware ID** on the Device detail page. New "Batch" row
  (only when populated; e.g. `2430-Broadwell`) and "Firmware ID" row
  (full identifier including build hash, e.g.
  `20260423-102547/2.0.0-beta1-g8c7700a`). Backed by two new columns
  on the devices table (migration 024). Captured opportunistically:
  `batch` from `Shelly.GetDeviceInfo` during firmware checks, `fw_id`
  from both `/shelly` (scanner / refresh) and `Shelly.GetDeviceInfo`
  (firmware check). Empty for existing rows until the next probe.
- **`Firmware.Result` carries Batch + FWID** so the firmware-check
  job can persist them in the same write that updates the per-channel
  cache; no extra RPC.

### Changed
- **Devices page Model column sort** now keys on the displayed text
  (app code first, model SKU fallback) so a click on the column
  header matches what the eye sees in each cell. Mirrors the
  Firmware-page comparator from v0.1.11.
- `Store` interface gains `GetLogsFiltered` and
  `GetLogsForExportFiltered`; the un-filtered legacy methods stay as
  thin wrappers.

### Migration notes
- Migration `024_device_batch_fwid.sql` adds two `TEXT NOT NULL
  DEFAULT ''` columns. Empty for existing rows; populated by the next
  scan / refresh / firmware check on each device.

## [0.1.11] - 2026-05-07 — Friendly device labels (app code) + detail-page enrichment

Surfaces Shelly's short application code (`PlugSG3`, `Pro4PM`,
`Pro3EM`, …) as the primary "what is this device" label across the
Devices and Firmware pages, with the canonical SKU and component
counts moved into the hover tooltip. The detail page gains a header
badge plus Type / Model SKU / Components rows in the Status grid.
Includes migration `023_device_app.sql`.

### Added
- **`app` field on the Device row.** Sourced from the `app` key
  Shelly's `/shelly` endpoint already returns (no extra RPC). Carried
  through scan and refresh paths automatically. Empty until the next
  scan / refresh on devices that pre-date this release.
- **Devices and Firmware page Model columns** show the friendly app
  code as the cell text. Hover tooltip lists App + Model SKU + Gen +
  switch / cover / light component counts in one place.
- **Device detail page**: small grey badge after the device name
  (with the app code), plus three new rows in the Status grid — Type
  (friendly app label), Model SKU (canonical code), Components
  (humanized count: "4 switches, 1 cover").
- **Firmware page Model-column sort** now keys on the displayed text
  (app first, model code fallback) so a click on the column header
  groups devices by their friendly type instead of by SKU.

### Migration notes
- Migration `023_device_app.sql` adds an `app TEXT NOT NULL DEFAULT ''`
  column to `devices`. Empty for existing rows; populated on the next
  scan or refresh.

## [0.1.10] - 2026-05-07 — Capabilities column + structured risk_level on audit log

Two small operator-facing improvements that together close out the
ADR-0010 first wave: a Capabilities column on the Devices list, and a
structured risk_level attribute on every audit row written for an action
execution. Includes migration `022_audit_log_risk_level.sql`.

### Added
- **Capabilities column** on the Devices page (toggleable via Columns,
  off by default). Per-row badges show switch / cover / light component
  counts derived server-side from `RawStatus` in `GetDevices()`. Lets
  operators spot at-a-glance which devices expose which component types
  without opening the device-detail page.
- **`risk_level` on audit rows.** New TEXT column populated only for
  action-execution rows (`low` / `medium` / `high`, sourced from the
  catalog risk in `actions.go`). Empty on every other audit row.
  Threaded via a context-bound helper (`services.WithRisk`) so adding
  the field didn't cascade through every call site that uses
  `LogCtx`. Backed by a new `db.AddLogWithAttrs` write surface; the
  legacy `AddLog` / `AddLogWithRequestID` are unchanged thin wrappers.
- **Logs page Risk column** — small badge rendering of `risk_level`
  alongside the existing Level / Request / Message columns. Empty for
  non-action rows so the visual noise stays minimal.
- **CSV log export gains a `risk_level` column** between `level` and
  `request_id`. NDJSON export already round-trips the field naturally
  via the `LogEntry` JSON tags.
- The structured `risk_level` attribute also lands in the slog JSON
  mirror (visible in container logs via `docker logs`), so an operator
  grepping container output can filter the same way SQLite queries do.

### Migration notes
- Migration `022_audit_log_risk_level.sql` adds a single
  `risk_level TEXT NOT NULL DEFAULT ''` column to `audit_log`. Existing
  rows get empty values; no backfill needed.

## [0.1.9] - 2026-05-07 — Per-component action fan-out (cover/switch/light) + OTA revert

Completes the v2 wave of [ADR-0010](docs/adr/0010-per-device-action-discovery-via-listmethods.md):
the action catalog now expands one entry into N per-component-instance
actions, so a Pro 4 PM gets four `Switch N — Toggle` rows automatically.
Adds `ota_revert` (firmware rollback) gated by the typed-name confirm
modal alongside the factory-reset variants. No schema migration.

### Added
- **Per-component action fan-out.** Catalog entries can now declare
  `component: "switch" | "cover" | "light"` and `describeAvailableActions`
  expands one entry into one action row per `<component>:N` instance the
  device exposes via `RawStatus`. Action IDs gain a `:N` suffix; the
  ExecuteDeviceAction dispatcher peels it off and threads the integer
  through `DeviceActionRequest.Instance`.
- **Six new component-bound actions** (only appear on devices that
  actually expose the component + its RPC):
  - `switch_toggle:N` — flip a switch on/off (`Switch.Toggle`)
  - `light_toggle:N` — flip a light on/off (`Light.Toggle`)
  - `cover_open:N` / `cover_close:N` / `cover_stop:N` — drive a cover
    (`Cover.Open` / `Cover.Close` / `Cover.Stop`)
- **`ota_revert`** action (`OTA.Revert`) — firmware rollback. High-risk;
  protected by the typed-name confirm modal alongside the factory-reset
  variants. Useful when a recent firmware update introduces a regression.
- **`Instance` field on `DeviceActionRequest`** so component-bound Apply
  functions don't need to re-parse the action ID.
- New tests: per-component fan-out, instance discovery from `RawStatus`,
  the `<base>:<instance>` parser. Dispatch table coverage extended to all
  v0.1.8 + v0.1.9 actions.

### Changed
- ADR-0010 promoted from `docs/plans/broader-action-discovery.md` to
  `docs/adr/0010-per-device-action-discovery-via-listmethods.md` with
  status `Accepted`. References in CHANGELOG / `actions.go` / ARCHITECTURE
  follow the new path.

## [0.1.8] - 2026-05-07 — Per-device action discovery via Shelly.ListMethods

Replaces the hand-rolled five-action surface with a declarative catalog
filtered against each device's `Shelly.ListMethods` output. Adds four new
actions (Wi-Fi scan, Ethernet status read, "reset Wi-Fi & cloud", full
factory reset) and a typed-name confirm modal for the two unrecoverable
ones. See [ADR-0010](docs/adr/0010-per-device-action-discovery-via-listmethods.md)
for the design. Includes migration `021_device_supported_methods.sql`.

### Added
- **Per-device cached method list.** New `supported_methods` column on
  the `devices` row holding the device's `Shelly.ListMethods` output as
  a JSON array. Populated on every firmware-check and refresh; nil
  means "never probed" and the action layer falls back to the v0.1.7
  catalog so the rollout window leaves no device action-less.
- **Declarative action catalog** in `internal/services/actions.go`.
  Each entry declares `RequiredMethods []string` plus an `Apply`
  function; discovery is a filter, not a hand-edited switch.
  `ExecuteDeviceAction` becomes a single dispatch.
- **Four new per-device actions**, gated by their respective RPC
  methods:
  - `wifi_scan` (`Wifi.Scan`) — list visible SSIDs, useful for diagnostics.
  - `eth_status` (`Eth.GetStatus`) — read live link/IPv4/IPv6 status.
  - `factory_reset_wifi` (`Shelly.ResetWiFiConfig`) — clear stored Wi-Fi + cloud config; preserves scripts/KVS/schedule.
  - `factory_reset` (`Shelly.FactoryReset`) — wipe all persisted configuration.
- **Typed-name confirm modal** for `factory_reset` and
  `factory_reset_wifi`. Operator must type the device's name exactly
  before the RPC fires. Reversible high-risk actions (`firmware_update`,
  `reboot`) keep the existing single-click behaviour. ADR-0002 carve-out
  documented in the plan.
- **Risk-grouped action ordering** — the API now returns actions
  sorted low → medium → high so the front-end renders a natural
  click-freely → confirm-required progression.
- New tests: `methodsCovered` branches, fallback-when-unprobed,
  filter-by-methods, online-gate, risk ordering, dispatch table.

### Changed
- `BLE.Pair` action's runtime "skipped on unsupported firmware" path
  now functions as a fallback: once `SupportedMethods` is populated the
  catalog filter handles it before the RPC fires. Old behaviour kept
  for the rollout window.
- `app_clients.refreshFirmwareCache` renamed to
  `refreshDeviceCapabilities` and extended to also pull
  `Shelly.ListMethods`. Three RPCs per online device per refresh
  (CheckForUpdate + ReadAutoUpdate + ListMethods); negligible
  wall-time at concurrency-64 fleet sizes.

### Migration notes
- Migration `021_device_supported_methods.sql` adds a single
  `supported_methods TEXT NOT NULL DEFAULT ''` column. Empty string =
  "never probed", populated by the next firmware-check or refresh on
  each device.

## [0.1.7] - 2026-05-06

Scheduled firmware checks, configurable install timeout, the legacy
firmware/credential column drop, and a CI auto-release pipeline. No
operator-visible change to existing workflows; the new surfaces are
opt-in via Settings → Firmware. Includes migrations
`019_drop_legacy_fw_columns.sql` and
`020_drop_legacy_credential_columns.sql`.

### Added
- **Scheduled firmware checks** — new `firmware_check_interval` setting
  (seconds, 0 = disabled). A long-lived background goroutine polls
  AppSettings every 60 s, fires `StartFirmwareCheck` at the configured
  cadence, and skips ticks when a manual check is already running.
  Settings UI exposes presets: Off / Hourly / 6h / 12h / Daily / Weekly.
- **Configurable per-device install timeout** — new
  `firmware_install_timeout` setting (seconds, default 300). Replaces
  the previous hardcoded 5 min. Per-device, not job-total. Surfaced in
  the timeout detail line ("device still on X after 8 min" etc.).
- **Auto-release on tag push** —
  `.github/workflows/publish-image.yml` now extracts the matching
  CHANGELOG entry via awk and calls `gh release create` (or
  `gh release edit` if the release was hand-created), so future `v*`
  tag pushes produce a GitHub Release alongside the GHCR image. Tags
  matching `*-rc*` / `*-beta*` / `*-alpha*` are auto-marked as
  prereleases.
- Unit tests for `firmwareInstallTimeoutFromSettings`,
  `firmwareSchedulerDecision`, and `formatTimeout` covering all
  reachable branches (`internal/services/app_jobs_test.go`).

### Changed
- **Migration `019_drop_legacy_fw_columns.sql`** drops `fw_status` and
  `fw_available_ver` from `devices`. Both columns have been
  unread/unwritten by Go code since v0.1.5; the rollback window is
  closed.
- **Migration `020_drop_legacy_credential_columns.sql`** drops
  `password` and `ha1` from both `credentials` and `credential_groups`.
  These columns have been zeroed at every boot since v0.0.15 (the
  one-shot encryption sweep). Cipher columns
  (`password_cipher`, `ha1_cipher`) are now the only at-rest source.
  The `encryptPlaintextCredentials` sweep is removed; `resolveSecret`
  is replaced by a trivial `decryptCipher` helper.
- `.gitignore` now excludes `.claude/` so the local CLI working
  directory stops appearing in `git status`.

### Migration notes
- SQLite `ALTER TABLE DROP COLUMN` is in-place but does not VACUUM the
  file. Plaintext bytes from pre-v0.0.15 installs may remain on disk
  pages until SQLite recycles them. Operators with strict scrubbing
  requirements should run
  `sqlite3 ${DATA_DIR}/shellyctl.db "VACUUM"` once after upgrade.
- Downgrade below v0.1.7 is not supported on installs that have run
  migrations 019/020. The dropped columns can't be recovered without a
  pre-upgrade backup.

## [0.1.6] - 2026-05-06

Adds firmware **auto-update** support via the device's `Schedule.*` API (the
same mechanism the device's own web UI uses) and surfaces it across the
Firmware, Devices, Compliance, and Provision pages. Refresh is extended to
also sync firmware availability + auto-update mode, so it's now the single
"make this row reflect reality" button. Includes migration
`018_device_fw_auto_update.sql`. CI: migrated to golangci-lint v2 +
`golangci-lint-action@v9` ahead of the 2026-06-02 Node 20 sunset.

### Added
- **`fw_auto_update` per-device** (`""` | `off` | `stable` | `beta`).
  Shelly Gen2+ exposes no dedicated OTA-config method; the local web UI
  implements its "Enable auto update firmware" toggle (FW 1.2.0+) as a
  `Schedule.Create` job that calls `Shelly.Update` with
  `origin: "shelly_service"`. ShellyAdmin honours the same marker so
  user-created Schedule entries are not clobbered. New helper module
  `internal/core/firmware/autoupdate.go`.
- **Bulk auto-update buttons** on the Firmware page: `Auto → Off / Stable
  / Beta` operate on the row selection. Row checkboxes are now
  channel-agnostic (no longer disabled when a device is already on the
  latest firmware), so any device can be a target. `Update N` still
  filters internally to install-eligible rows.
- **`set_auto_update` bulk action** in the device-action API
  (`POST /api/bulk` with `value: "off"|"stable"|"beta"`).
- **Auto-Update column** on both the Firmware and Devices pages
  (toggleable on Devices via Columns).
- **Compliance rule** `auto_update_stage` in its own SectionCard
  "Auto-Update Schedule (Gen 2+, FW 1.2.0+)". Devices whose schedule
  doesn't match flag as non-compliant. Skipped on devices not yet
  firmware-checked so mixed fleets don't false-positive.
- **Provision template section** `auto_update`. Canonical bare-string
  encoding (`"auto_update": "stable"`); also accepts
  `{stage: "stable"}`. New "Auto-Update Schedule (Gen 2+)" section in
  the Provision Misc form. Backend handler in `applySection`.

### Changed
- **Refresh now syncs firmware data.** Per-device Refresh (Devices page)
  and bulk Refresh both call `Shelly.CheckForUpdate` + `Schedule.List`
  per online device, updating `fw`, `fw_available_stable`,
  `fw_available_beta`, `fw_checked_at`, `fw_auto_update` in the same
  pass. Refresh stops being a "data subset" relative to Check Firmware.
- **Refresh no longer blanks the firmware cache.** Latent regression:
  `scanner.ProbeDeviceWithOptions` constructed a fresh Device that
  zeroed the firmware fields. The fix carries the persisted values
  forward before the firmware re-check writes fresh ones, so a
  transient cloud blip during refresh leaves your cache intact.
- **Compliance: Auto-Update rule lives in its own SectionCard** between
  Zigbee and "FW 2.0+ checks". Previously nested inside the 2.0+
  section, which misled operators since Schedule.* works on FW 1.2.0+.
- **CI: golangci-lint v1.64 → v2.6**; `.golangci.yml` migrated via
  `golangci-lint config migrate`; action `@v6` → `@v9` (Node.js 24).
- **Bundle budget** bumped to 280 kB raw / 80 kB gzip to absorb the
  v0.1.5+v0.1.6 surface additions.

### Fixed
- Row checkboxes on the Firmware page no longer auto-uncheck themselves
  on channel toggle — devices on the latest firmware can now be
  selected for the auto-update bulk actions.
- Bulk auto-update status message moved out of the toolbar into a
  dismissable inline notice between toolbar and progress bars.

### Migration notes
- Migration `018_device_fw_auto_update.sql` adds the `fw_auto_update`
  column. Empty default = "never read"; populated by the next firmware
  check or refresh on each device.

## [0.1.5] - 2026-05-06

Full rebuild of the firmware update page: dual-channel availability
cache, dedicated install-progress job, and confirmation modal — driven
by a `/grill-me` design pass after multiple bug reports against the
v0.1.4 page. Adds migration `017_device_fw_per_channel.sql`.

### Added
- **Per-device, per-channel firmware cache** on the Device row:
  `fw_available_stable`, `fw_available_beta`, `fw_checked_at`.
  `Shelly.CheckForUpdate` returns both stable and beta sections in a
  single response; we now persist both. The Firmware page channel
  selector becomes a pure display + install filter — toggling is
  instant with no re-check.
- **Dedicated `firmware_install` job** replacing the fire-and-forget
  `Shelly.Update` path. Bounded concurrency (5 in flight), per-device
  polling of `Shelly.GetDeviceInfo` every 5 s until version match,
  hard 5-min timeout per device. New `GET /api/firmware/install/status`
  surfaces live progress.
- **Confirmation modal** on bulk update — lists affected device names,
  IPs, and target version, plus the channel, before any RPC fires.
- **Sortable Firmware table.** Click any column header (Name, Gen,
  Model, IP, Current, Available Stable, Available Beta, Status). IP
  sorts numerically by octet.
- **Select-all checkbox** in the table header with indeterminate state
  when only some rows match.
- **Configurable Gen 2 / 3 / 4 badge colors** (Settings → UI
  Preferences). Seven-preset Bootstrap palette with live preview.
- **Shared Stable/Beta channel store** (persisted to localStorage) so
  toggling on the Firmware page also takes effect on the Devices page
  and vice versa.
- **Out-of-band firmware drift detection.** Every firmware check now
  also calls `Shelly.GetDeviceInfo` and writes the running version
  back to `Device.FW`, so devices upgraded via the device's own web UI
  reflect their current firmware in ShellyAdmin without needing a
  separate Refresh.
- **Firmware columns on the Devices page** (`fw_available_stable`,
  `fw_available_beta`) so update availability is visible from the
  primary list, not just the Firmware page.

### Changed
- **`firmware.Result` rebuilt to dual-channel**: `stable_ver`,
  `beta_ver`, `stable_update`, `beta_update`, `status`, `note`,
  `checked_at`. Old `update_available` / `available_ver` / `stage`
  fields removed. `pickStage` / `stageNote` helpers deleted — they
  silently fell back to the other channel and caused wrong-channel
  ghost updates.
- **Friendlier RPC errors.** Timeouts → "device did not respond in
  time", connection failures → "connection refused" / "no route to
  host", DNS failures → "DNS lookup failed". Anything unrecognized is
  truncated to 120 chars instead of dumping a raw Go stack into the
  status detail line.
- **Firmware install timeout message** now describes what actually
  failed: "device still on 1.7.5 after 5 min (expected 1.8.99)"
  instead of the previous "did not come back in time".
- **Per-device firmware check timeout** bumped 5 s → 10 s.
- **Stale install overlay clears on the next firmware check**, so a
  fresh check meaningfully resets the page.

### Fixed
- **`selected` Set reactivity** — Svelte 4 doesn't track `.add()` /
  `.delete()` mutations, so the "Update N" counter never updated
  ([previous behaviour: counter stayed at 0]). Replaced the Set with
  `bind:group` against a `string[]`. Also fixes the "counter shows 1
  but no row checked" stale-MAC bug, and the "still selected after
  channel toggle" bug.
- **Wrong-channel ghost updates** when stable was selected but only
  beta had an update — `pickStage` would silently fall through, the
  row got marked updateable, then `Shelly.Update` was called with
  `stage: stable` and silently no-op'd.
- **Status badge stuck on "update" forever** after a successful
  install — now flips to "current" automatically once the device
  reboots onto the new firmware.
- **Missing name + model** on the Firmware page rows; **non-clickable
  IP**. Both addressed in the new column layout (clickable IP opens
  the Shelly device's own web UI in a new tab).

### Migration notes
- Migration `017_device_fw_per_channel.sql` adds three columns
  (`fw_available_stable`, `fw_available_beta`, `fw_checked_at`).
  Existing rows get empty defaults until the next firmware check
  populates them. Legacy `fw_status` / `fw_available_ver` columns are
  left in place but no longer read or written.

## [0.1.4] - 2026-05-04

UX: remove the section-level "enable section" checkbox from every form on
the Provision page. Now matches the Compliance page behaviour — sections
auto-expand whenever any inner field toggle is on. Fixes the long-standing
confusion where ticking the section header then ticking inner toggles made
the section feel impossible to collapse.

### Changed
- **`SectionCard` `enabled` prop is no longer passed by Provision forms**
  (Sys, Mqtt, Ws, Ble, Matter, Cloud, Auth, Wifi (and sta/sta1/roam),
  WifiAP, Modbus, Zigbee, Eth, UI, Scripts). The `enabled: boolean` field
  was removed from each State type, and the `if (!s.enabled) return null`
  early-return in every `build*` function was dropped — sections are
  emitted whenever they have at least one inner field set, exactly the
  same logic that already gated each individual field.
- **Inner-field `disabled={!state.enabled || ...}` guards removed.** Each
  inner FieldRow / Toggle / input now disables purely on its own
  `*Enabled` flag.
- **Hydration no longer sets `state.enabled = true`** when a saved
  template loads — the inner fields determine visibility.

### Operational note
Saved templates continue to load correctly. The on-wire JSON shape is
identical to v0.1.3 for sections that have ≥ 1 field set; sections that
were "enabled but empty" in v0.1.3 (which sent `{}` to the device, a no-op
in practice) are now omitted entirely.

## [0.1.3] - 2026-05-04

Third patch fix for the v0.1.0 scanner false-positive issue. v0.1.1
caught empty bodies, v0.1.2 caught Basic-auth 401s, and v0.1.3 catches
the **HTTP→HTTPS redirect to a self-signed cert** path used by UniFi
UDM Pro Max — the device redirects HTTP `/shelly` to HTTPS, the TLS
handshake fails on the self-signed cert, and ShellyAdmin was converting
the resulting `ErrTLSCertInvalid` into a partial Device record. This
patch generalises the fix so any recoverable probe failure on an
unknown IP is dropped, not persisted.

### Fixed
- **Probe failures during scan no longer create partial Device records.**
  `scanner.ProbeOptions` gained a `KnownMAC` field. When set (refresh
  path), recoverable failures (auth-required, lockout, TLS-cert-invalid)
  produce a partial record carrying that MAC so the existing device row
  stays accurate. When empty (scan-of-unknown-IP path), recoverable
  failures yield `nil` — without positive Shelly evidence we have
  nothing to persist (`internal/core/scanner/scanner.go`,
  `internal/services/app_clients.go`).

### Tests
Regression coverage for: HTTP→HTTPS redirect to a self-signed cert
(UDM Pro Max shape), Basic-auth 401 at the scanner layer (both with
empty `KnownMAC` should yield nil; with non-empty `KnownMAC` should
yield a partial record).

## [0.1.2] - 2026-05-03

Second patch fix for the v0.1.0 scanner false-positive issue: v0.1.1
caught the empty-body and non-Shelly-JSON cases but missed the most
common UniFi case — UDM Pro / Protect cameras return `401 Unauthorized`
with `WWW-Authenticate: Basic realm="..."` on `/shelly`. v0.1.1's auth
handler treated *any* 401 as "Shelly with creds needed" and returned
`ErrAuthRequired`, which the scanner converted into a partial Device
record with empty MAC. Even though the MAC primary key dedupes them at
write time, they still flashed in the live scan-progress UI.

### Fixed
- **Non-Digest 401 responses no longer surface as `ErrAuthRequired`.**
  A real Shelly always uses RFC 7616 Digest auth; a 401 with `Basic`,
  `Bearer`, or no `WWW-Authenticate` challenge means the endpoint isn't
  Shelly. `shellyclient.do` now returns a generic descriptive error in
  that case, which `reportProbeFailure` ignores — no partial record
  created, IP skipped cleanly (`internal/core/shellyclient/client.go`).

### Other fixes
- **Power-readings voltage was order-dependent (flake).** The
  `extractPowerReadings` function clobbered `lastV` for switch / pm1 /
  em1 components but took max for em (3-phase). Go map iteration order
  is randomized, so `TestExtractPowerReadings_SwitchAndEM` would fail
  ~50 % of the time. Logic now consistently takes the maximum non-zero
  voltage across all components (`internal/core/scanner/scanner.go`).
- Test failure messages now dereference `*float64` pointers so the
  output reads "VoltageV = 230, want 232" instead of a hex address.

## [0.1.1] - 2026-05-03

Patch fix for a v0.1.0 scanner regression: non-Shelly endpoints (UniFi
UDM Pro / Protect cameras, generic web servers, captive portals) that
answered `GET /shelly` with `200 OK` and an empty / non-Shelly JSON body
were being persisted as junk Device records with empty MAC and an
arbitrary Gen=2 default.

### Fixed
- **Scanner now rejects non-Shelly responses on `/shelly`.** A real
  Shelly always reports either a non-empty `mac` or a non-zero `gen`
  field; without one of those the IP is logged at DEBUG and skipped
  (`internal/core/scanner/scanner.go`, `internal/core/shellyclient/client.go`).
- `shellyclient.Probe` now treats an empty 200 body as an error rather
  than returning an empty map. The pre-v0.1.0 code did this implicitly
  via `json.Decoder.Decode` returning `io.EOF`; the v0.1.0 rewrite to
  `io.ReadAll` + `json.Unmarshal` lost that guard.

### Operational note
Existing junk Device rows (created by v0.1.0 scans before this fix) need
to be cleaned up manually via the Devices page row-level remove button —
the migration does not retroactively delete them. A subsequent scan on
v0.1.1 will not re-create them.

## [0.1.0] - 2026-05-03

Adapt ShellyAdmin to Shelly Gen2+ firmware **2.0.0-beta1**. The release adds an
RFC 7616 Digest auth client, HTTPS scheme handling with per-device TLS policy,
strips the removed BLE `enable` flag, surfaces new compliance fields
(enhanced_security, tls_cert_valid, wifi_hostname), and exposes the per-device
WiFi hostname in the provisioner UI. Includes one schema migration
(`015_device_fw2_fields.sql`).

### Added
- **`internal/core/shellyclient`** — unified HTTP/JSON-RPC client used by every
  device-talking code path (scanner, provisioner, setters, firmware). Implements
  RFC 7616 Digest auth (SHA-256 with MD5 fallback, qop=auth, nonce-counter
  reuse), 429 brute-force-lockout signalling via `ErrAuthLockout`, and
  configurable TLS policy. Old call sites kept their timeout-only signatures via
  back-compat wrappers; new `*WithOptions` variants thread credentials and
  scheme through per device.
- **Device fields** for FW 2.0 state: `Scheme`, `EnhancedSecurity`,
  `TLSCertValid`, `TLSAllowInsecure`, `AuthLockedUntil`, `WiFiHostname`,
  `WiFiChannel`. Migration `015_device_fw2_fields.sql` adds the columns;
  existing rows get `scheme="http"` and null TLS/EnhancedSecurity.
- **Compliance rules**: `enhanced_security`, `tls_cert_valid`, `wifi_hostname`,
  `ble_paired`, `webhooks_configured`. Mixed-fleet safe — rules are skipped
  when the device hasn't reported the underlying state.
- **Per-device credential lookup** for refresh/firmware/setter paths via the
  existing credential-group → credential pipeline. Bulk actions now run with
  the device's assigned credential automatically.
- **WiFi hostname** field in the provisioner STA form, hydrated from saved
  templates. Routes through `Wifi.SetConfig`'s native `sta.hostname`.
- **Cover provisioner section** (`case "cover"`) with normalizer hook for the
  slat/tilt config introduced for venetian-blinds support.
- **Cover.GoToTilt setter** for slatted-cover bulk control.
- **Webhooks provisioner section** (`case "webhooks"`) — declarative
  delete_all → delete → update → create pipeline driving Webhook.* RPCs.
  Method-not-found errors on older firmware surface as "skipped" so mixed-fleet
  templates don't blow up.
- **LNM provisioner section** (`case "lnm"`) — explicit handler so the
  all-caps `LNM.SetConfig` method routes correctly (the catch-all would
  produce `Lnm.SetConfig`).
- **BLE pair device action** — new per-device action `ble_pair` that calls
  `BLE.Pair`. Surfaces "skipped" on firmware that doesn't expose the RPC.
- **Live power telemetry** (Phase C1+C4): scanner extracts `apower`,
  `voltage`, `current` from switch/em/em1/pm1 status components and sums them
  into device fields `PowerW`, `VoltageV`, `CurrentA`. Surfaced on the
  Devices list (sortable columns) and a "Live Readings" card on DeviceDetail.
- **Compliance UI** for the firmware 2.0 fields — new SectionCard with toggles
  for `enhanced_security`, `tls_cert_valid`, `wifi_hostname`, `ble_paired`,
  `webhooks_configured`. Mixed-fleet safe; rules are skipped when the
  underlying state isn't reported.
- **Migration `016_device_power_readings.sql`** adds `power_w`, `voltage_v`,
  `current_a` columns.

### Changed
- **BLE `enable` flag stripped at provisioning time** with a per-device warning,
  matching FW 2.0.0-beta1 which removed the flag (BLE auto-activates with
  scanning). The toggle is removed from the provisioner BLE form; older
  templates that still set `ble.enable` continue to load with a console warning.
- **Provisioner / setters / firmware / scanner** route every RPC through
  `shellyclient.Client` instead of bare `http.Client`. Method-not-found is now
  detected via the typed `RPCError` rather than ad-hoc body parsing.
- **Refresh path** distinguishes recoverable failures: `AuthRequired`,
  `AuthLockedUntil`, and `TLSCertValid=false` partial probes update the
  device row without flipping it offline.

### Security
- Authenticated probing means devices behind admin auth (FW 1.x and 2.0+)
  no longer silently appear offline; we now actually authenticate when a
  credential is mapped to the device.
- HTTPS scheme awareness: when a 2.0 device redirects HTTP→HTTPS (with
  `enhanced_security` enabled), the scheme is remembered and reused on
  subsequent calls. Per-device `tls_allow_insecure` opt-out for self-signed
  certs.

## [0.0.16] - 2026-04-24

Follow-up release after a combined `/review` and `/security-review` of the
v0.0.7 → v0.0.15 window. Closes one medium-severity provisioning leak, finishes
the v0.0.14 OTA removal across the UI, surfaces bulk-refresh auth failures,
redirects the SPA on expired sessions, plumbs request IDs into service-layer
logs, tightens settings validation, and dedupes helpers the earlier passes left
behind. No schema migrations.

### Security
- Remove undocumented `${ENV:...}` env-var expansion from provisioning template
  substitution (`internal/core/provisioner/provisioner.go`). The feature allowed
  an authenticated admin to exfiltrate server env vars (including
  `SHELLYADMIN_PASS_HASH`, `SHELLYADMIN_SECRET`, `SHELLYADMIN_ENCRYPTION_KEY`)
  by POSTing a crafted template to an attacker-controlled LAN IP. Only the
  documented `{device_name}` token remains.

### Fixed
- **Bulk refresh now surfaces `AuthRequired` / `AuthError`** the same way the
  single-device path does, so a password-mismatch device no longer shows a
  generic "refresh timed out" row (`internal/services/app_jobs.go`).
- **`debug.mqtt` passthrough in provisioning templates** — the toggle existed
  in `SysForm.svelte` but the backend normaliser dropped it; it is now
  preserved alongside `debug.websocket` and `debug.udp`
  (`internal/core/provisioner/provisioner.go`).
- **SPA redirects to `/login` on 401** for expired sessions instead of showing
  an opaque error (`web/src/lib/api.ts`).
- **`session.Save()` errors are no longer swallowed** on login or logout; login
  now returns 500 on persistence failure instead of handing the user a broken
  cookie (`internal/api/handler.go`).
- **Compliance custom-rule regex is validated at save time**; a bad pattern no
  longer silently classifies every device as non-compliant
  (`internal/services/app.go`, `internal/core/compliance/compliance.go`).
- **Lat/Lon bounds (`±90` / `±180`) enforced in `ValidateSettings`** so invalid
  compliance settings are rejected on save, not only when a bulk action runs
  against them (`internal/services/app.go`).

### Changed
- **OTA removal finished across the UI and backend** (started in v0.0.14).
  Frontend: dropped the Provision → Misc "OTA" section, the `OtaState` type,
  and `ota_auto_update` from the Compliance page. Backend: dropped
  `OTAAutoUpdate` from `models.AppSettings` / `ComplianceRules`, the OTA branch
  in `applySection`, and the `normalizeOTAPayload` helper. The `ota` key is
  still accepted as a passthrough via the default handler (it 404s on the
  device and is skipped gracefully) to avoid breaking anyone with existing
  template JSON.
- **Request IDs propagate into service-layer logs.** The service log callback
  now takes `context.Context`; request-scoped sites (Provision, UploadUserCA,
  BulkAction, ExecuteDeviceAction, RefreshDevices, Stop) log via a new
  `LogCtx` helper so the ID populates both the `audit_log.request_id` column
  and the slog JSON attribute. Background jobs (scan, firmware, recovery)
  intentionally keep the no-ctx path since they outlive the HTTP request
  (`internal/services/app.go`, `internal/services/app_jobs.go`,
  `internal/services/device_surface.go`, `internal/api/handler.go`,
  `cmd/shellyctl/main.go`).
- **`AppService.Stop` is idempotent** via `sync.Once` so overlapping signal
  handlers can't re-cancel or re-mark interrupted jobs.
- **Shared `sslCAOptions` dropdown** for the Provision MQTT and WebSocket
  forms. `WsForm` previously took a freeform input; both now use the same
  four-option Select (`""`, `*`, `ca.pem`, `user_ca.pem`) matching the only
  values the Shelly API accepts (`web/src/pages/provision/sslCa.ts`).
- **`firstNonEmpty` deduped** into a single exported `util.FirstNonEmpty`
  (trim-variant) used by the provisioner, service, and user-CA code paths
  (`internal/util/strings.go`). Removed the dead `boundedConcurrency` alias
  and the dead `gen int` parameter on every setter in `internal/core/setters`.

## [0.0.15] - 2026-04-22

Security-hardening round: argon2id admin password hashing, encryption at rest for
device credentials, request correlation IDs across the audit trail, sanitized HTTP
error responses, and Svelte lint re-enabled in CI. No user-facing workflow changes;
all migrations are forward-only and transparent on first boot.

### Added
- **Argon2id admin password hashing** via a new `SHELLYADMIN_PASS_HASH` env var
  (also honours `_FILE`). Generate the PHC string with `shellyctl hash-password
  <plaintext>` (a new subcommand on the same binary, also runnable via
  `docker run --rm ghcr.io/buliwyf42/shellyadmin:latest shellyctl hash-password …`).
  Plaintext `SHELLYADMIN_PASS` still works for backward compatibility but emits a
  deprecation warning on startup; removal planned for a future release.
- **Encryption at rest for device credentials** (`credentials` and
  `credential_groups` `password` / `ha1` columns). XSalsa20-Poly1305 via
  `nacl/secretbox` with a 32-byte key resolved from `SHELLYADMIN_ENCRYPTION_KEY`
  (base64, `_FILE` suffix supported) or generated on first boot at
  `${DATA_DIR}/shellyadmin.key` (0600). Migration 013 adds cipher columns and a
  one-shot sweep on startup rewrites any legacy plaintext rows. **Back the key
  file up alongside the database** — losing it permanently orphans every stored
  credential.
- **Request correlation IDs** (`X-Request-ID`) generated by a new
  `internal/middleware/requestid` middleware: 16-hex per request, echoed on every
  response, honours client-supplied IDs (alnum/`-`/`_`, truncated to 64 chars).
  Stashed on both gin and stdlib contexts. Audit entries persist the ID in a new
  `audit_log.request_id` column (migration 014); the Logs page surfaces it and CSV
  export includes the new field.
- **Structured slog mirror** for audit lines. Operator-tailing the container log
  now sees JSON records with `request_id` attributes alongside the SQLite-backed
  audit trail.
- **Scanner network allowlist + tighter CIDR cap** (`internal/core/scanner`):
  targets must be RFC1918 or link-local, loopback/multicast/public ranges are
  rejected, and max CIDR size drops from `/16` (65K hosts) to `/22` (1024 hosts).
- **`Store` interface at the service/DB boundary** (`internal/services/store.go`),
  enabling unit tests against a fake persistence layer without spinning up SQLite.
- **Per-device bulk-action audit detail**: `summarizeBulkResults` now records
  ok / failed / skipped / missing counts and MAC lists (truncated at 20 + overflow)
  so a "what did this action touch?" question is answerable from the audit log alone.
- **CI lint and format gates** — `.golangci.yml` (govet, staticcheck, errcheck,
  ineffassign, unused, gofmt, goimports, misspell, unconvert) wired into the
  backend job; `web/eslint.config.js` (TS + `eslint-plugin-svelte`), `.prettierrc.json`
  (with `prettier-plugin-svelte`), and new `lint` / `format:check` scripts wired
  into the frontend job.
- **New documentation**: `docs/roadmap.md` as the source of truth for planned
  direction, linked from the README and the ADR index.

### Changed
- **Handler error responses are now sanitized**. Internal error details (stack
  traces, filesystem paths, DB quirks) are logged in full via the request-scoped
  audit path but never echoed to the client — 5xx responses return a generic
  `{"error": "<publicMsg>"}` body. User-facing validation errors still echo through
  via `respondUserError` so operator guidance (e.g. `scan_timeout must be
  positive`) is preserved.
- **Audit logging contract** now flows through a single `Handler.auditSink`
  pluggable for tests, replacing the previous scattered `logFn` method. Request
  ID is injected at the sink layer, so every `respondError` / `respondUserError`
  call automatically carries the correlation ID.
- **CSV log export** column order: `id, ts, level, request_id, message` (was
  `id, ts, level, message`).

### Security
- Admin password at rest is now a salted, memory-hard hash rather than plaintext
  when `SHELLYADMIN_PASS_HASH` is configured.
- Device credentials stored in SQLite are no longer readable from an offline DB
  copy (container escape, stolen backup, misconfigured volume). Does not defend
  against attackers with live process memory access.
- HTTP error responses no longer leak backend implementation details to
  authenticated callers.
- Scanning is constrained to local networks the operator is meant to be managing;
  accidental `0.0.0.0/0` or similar inputs are rejected.

### Migration notes
- Existing deployments will have `${DATA_DIR}/shellyadmin.key` generated on first
  boot under v0.0.15. Add this file to your backup rotation.
- Plaintext admin password continues to work; switch to the hash path at your own
  pace using `shellyctl hash-password`.
- Old legacy plaintext columns on `credentials` / `credential_groups` are retained
  for one release to keep a safe rollback window; they will be dropped in a
  follow-up migration.

## [0.0.14] - 2026-04-21

Bug fixes for the v0.0.13 UI improvements.

### Fixed
- **ProgressBar idle state** — bar now renders empty at 0/0 instead of showing a full-width gold fill (the fill div previously had no explicit width when `total=0`).
- **ProgressBar label split** — the "label below" span was inside `.pb-track` (which has `overflow:hidden`), causing the track to expand vertically and the fill to cover only the upper half. Span moved outside the track div.
- **OTA compliance rule always firing "unsupported"** — `OTA.SetConfig` does not exist on Shelly Gen2 devices so `ota.auto_update` is never present in any device config; the compliance check therefore always returned "unsupported" regardless of the configured rule. Removed the evaluation from the backend (`compareConfigStringOrUnsupported` helper and `OTAAutoUpdate` call), removed the OTA section from the Compliance settings UI, and cleared any previously saved value on next settings save.
- **OTA form radio buttons** — the "Update automatically" field in the Provision form and (previously) Compliance page used a `<Select>` dropdown; replaced with radio buttons matching the Shelly web interface (Disable / Enable stable / Enable beta, with an italic beta-instability warning).

## [0.0.13] - 2026-04-21

Completes the WiFi / provisioning / UI improvement sprint: full Wifi.SetConfig surface coverage (STA1, roaming, static IPv4), new Script / UI / Eth-IPv6 provisioner sections, per-device restart_required feedback after provisioning, reboot controls on the Devices page, and a shared polished progress bar component.

### Added
- **WiFi STA1 + roaming + static IPv4** provisioner coverage. The Provision form now exposes the full `Wifi.SetConfig` surface: a second station (`sta1`), roaming config (`rssi_thr`, `interval`), and per-STA static IPv4 fields (`ip`, `netmask`, `gw`, `nameserver`). New `WifiStaForm.svelte` and `WifiRoamForm.svelte` components reuse the existing field-enable-checkbox pattern.
- **Script, UI, and Eth-IPv6 provisioner sections**. `Script.SetConfig` per-id loop (mirrors the existing KVS handler), `UI.SetConfig` fields (`idle_brightness`, `debug_enable`), and Eth IPv6/DNS fields (`ipv6mode`, `ipv6` block). New `ScriptsForm.svelte` and `UIConfigForm.svelte` Provision-page forms; `EthForm.svelte` extended with a collapsible IPv6 section.
- **`restart_required` surfaced after provisioning**. Every `*.SetConfig` RPC response includes a `restart_required` flag. `SectionResult` now carries this field; `rpcSection()` parses it; the provision API response includes a device-level `restart_required` boolean. The Provision results view shows a gold **"restart required"** badge per device and a "Reboot N restart-required devices" button.
- **Reboot controls on the Devices page**. Per-row **⏻ reboot button** (confirm dialog, inline spinner, result notice) and a **Reboot All** toolbar button that reboots all currently-listed devices with a count-aware confirm. Both hit `POST /api/bulk` with `action: "reboot"`.
- **Bulk `reboot` action** wired into the backend (`validateBulkAction`, `applyBulkAction` → `setters.Reboot`, `bulkActionSummary`, `bulkActionWarnings`).
- **Shared `ProgressBar.svelte` component** (`web/src/components/`). Determinate mode: gold gradient fill with `width: 200ms` transition and animated 45° stripe overlay while running. Indeterminate mode (total = 0 while running): full-width stripe animation. Solid on completion, empty when idle at 0/0. Proper `aria-valuenow/min/max` when determinate, `aria-busy` while running, `aria-label` prop. Label inside fill when ≥ 25% wide, below otherwise.

### Changed
- Firmware and Scan pages now use `ProgressBar.svelte` instead of duplicated inline `<div class="progress">` markup. The unused `progress-bar-striped` class is gone.

### Fixed
- `SectionResult.RestartRequired` propagates correctly even when the section status is `ok` (previously the field was parsed but discarded before the return).

## [0.0.12] - 2026-04-21

Closes several Shelly API coverage gaps identified in the 2026-04 review: new compliance rules and provisioning surfaces for previously-unexposed device subsystems, plus chunked certificate upload that unblocks MQTT/WS with a user-managed CA and mTLS-to-broker auth.

### Added
- **User CA + TLS client cert/key upload** via chunked `Shelly.PutUserCA` / `Shelly.PutTLSClientCert` / `Shelly.PutTLSClientKey` RPCs. New `POST /api/provision/user-ca` endpoint (optional `kind` field: `user_ca` | `tls_client_cert` | `tls_client_key`) and a Provision-page **Upload Certificate (PEM)** form with a kind selector. Closes the MQTT/WS `user_ca.pem` loop and enables mTLS-to-broker authentication.
- **Per-IP concurrency guard** on certificate uploads — reuses the existing Provision/Firmware reservation pattern; uploads that collide with an in-flight Provision or Firmware job on the same device return a `skipped` result with a `device busy` detail instead of silently racing.
- **New compliance rules**: `wifi_ap_enabled`, `wifi_ap_is_open`, `eth_enabled`, `eth_ipv4mode` (`dhcp` | `static`), `sys_debug_mqtt`, `matter_enabled`, `modbus_enabled`, `zigbee_enabled`. Each rule surfaces in the Compliance page with the same toggle + enable-checkbox pattern as the existing rules.
- **New provisioner sections + UI forms**: `eth` (via `Eth.SetConfig`) and UI-only `wifi_ap`, `modbus`, `zigbee`, `user_ca` forms wired into the Provision page. The `eth` section joins the existing `mqtt`/`ws`/`wifi`/… section handlers in `applySection()`.
- Service-layer test suite `internal/services/user_ca_test.go` covering all input-validation paths (empty/too-many IPs, unknown kind, empty/headerless/oversized PEM, invalid/non-local IP) and a busy-target concurrency-guard case.
- Parameterized provisioner tests in `internal/core/provisioner/user_ca_test.go` exercising the chunked-upload sequence and back-compat wrapper across all three certificate kinds.

### Changed
- `internal/core/provisioner/user_ca.go` generalized around a `CertificateKind` enum (`KindUserCA` / `KindTLSClientCert` / `KindTLSClientKey`) with shared `UploadCertificate` / `RemoveCertificate` helpers. The original `UploadUserCA` / `RemoveUserCA` entry points are preserved as thin back-compat wrappers.
- `compliance.Evaluate` now honours the new rules via the existing `compareConfigBool` / `compareConfigString` helpers (no behaviour change for unset rules).
- `ComplianceRules.Normalize` coerces `eth_ipv4mode` to `dhcp` / `static` / empty — anything else is dropped rather than applied literally.

## [0.0.11] - 2026-04-18

Provision and Compliance UI refresh: dated `<select>`-based On/Off controls replaced with real toggle switches and a styled custom dropdown; section cards, field rows, and the Provision toolbar cleaned up. Plus a long-standing template-load bug fix.

### Added
- Four reusable form primitives under `web/src/components/`: `Toggle.svelte` (switch), `Select.svelte` (keyboard-navigable custom dropdown), `FieldRow.svelte` (enable-checkbox + label + control), and `SectionCard.svelte` (collapsible card with optional enable checkbox in the header). All are token-backed and reuse existing CSS variables (`--panel-2`, `--border`, `--warning`, `--radius-md`, `--control-height`) — no new dependencies.
- New token-backed component styles in `web/src/app.css` (`.sa-section`, `.sa-toggle`, `.sa-select`, `.sa-field`, `.sa-check`, `.sa-form-grid`, `.provision-toolbar`, `.sa-cluster`, `.sa-view-switch`).

### Changed
- Bulk actions `set_cloud_enabled` and `set_ble_enabled` (POST `/api/bulk`) toggle `Cloud.SetConfig {enable}` and `BLE.SetConfig {enable}` on the selected devices. Same preview / dry-run / per-target eligibility behavior as the existing toggles.
- Test coverage: `internal/core/setters` now has an `httptest`-backed unit test per setter (Sys, MQTT, Cloud, BLE, Reboot); `internal/db` has tests for `UpsertDevices` atomic commit, the two-miss offline transition, and error surfacing on a closed DB; `web/src/components/sortHeader.test.ts` covers the sort-direction derivation.
- `web/src/components/SortHeader.svelte` now derives its aria/indicator state from a small `sortHeader.ts` helper instead of inlining the logic — same behavior, but the derivation is unit-tested.
- Provision sub-forms (`SysForm`, `MqttForm`, `WsForm`, `BleForm`, `MiscForm`) and `Compliance.svelte` migrated to the new primitives. All On/Off `<select>` blocks replaced by Toggle; multi-value dropdowns (TLS mode, OTA stage, auto-update policy, custom-rule source/op) replaced by the custom Select; repeated "enable checkbox + label + control" markup now flows through FieldRow.
- Provision toolbar restructured into three visual clusters — template picker, save/rename, credential picker — replacing the previous single long strip of controls.

### Fixed
- Loading a template whose content the form can't represent (e.g. a `sys` section with unsupported keys) no longer wipes the form editor. `hydrateFormFromTemplate()` in `web/src/pages/Provision.svelte` is now atomic: each section is hydrated into a local variable first, and form state is only replaced when every section succeeds. On failure, the view still flips to JSON and a notice is shown — but switching back to Form preserves whatever was already entered.

### Removed
- Dead bulk action `set_24h` (was listed in `validateBulkAction` and `SortedBulkActions` but had no apply/summary path, so any client call silently fell through to "unsupported action").

## [0.0.10] - 2026-04-18

User-facing additions: per-device and per-job export flows, plus an "advanced mode" gate that hides the Provision JSON editor by default. CI also moves to Node-24 action majors ahead of the 2026-06-02 GitHub Actions Node 20 sunset.

### Added
- Settings: "Advanced mode" toggle (off by default). When off, the raw JSON template editor on Provision is hidden so the guided form is the only entry point. Flip it on in Settings → UI Preferences to expose the JSON tab.
- Per-device export endpoint `GET /api/devices/{target}/export` returning a JSON snapshot (`device`, `raw_config`, `raw_status`, `capabilities`). "Export JSON" button added to the device detail page.
- Audit log export endpoint `GET /api/logs/export?format=csv|ndjson` (CSV default, honours the same `level` + `search` filter as `/api/logs`, caps at 100k rows). "Export CSV" and "Export NDJSON" buttons added to the Logs page.

### Changed
- CI: bump GitHub Actions to Node 24–compatible majors (checkout v6, setup-node v6, setup-go v6, docker/* v4–v7) ahead of the 2026-06-02 Node 20 sunset.

## [0.0.9] - 2026-04-17

Review-closure release: closes all 11 findings from the 2026-04-17 project review — no user-facing feature changes, but meaningful reliability, structural, and hygiene improvements across backend and frontend.

### Backend reliability and structure
- Wrapped `UpsertDevices` in a single SQLite transaction so scan/refresh cycles leave the `devices` table consistent if the process is killed mid-loop.
- Replaced ~20 silent `_ = err` patterns in `internal/services/app.go` with explicit `log.Printf` calls so job finalization, JSON marshaling, and scan-payload parsing failures are no longer swallowed.
- Added a graceful-shutdown context to `AppService`: in-flight scan and refresh jobs now observe cancellation and are marked `interrupted` immediately on SIGTERM instead of waiting for the 15s/120s stale-job guard.
- Split `internal/services/app.go` (1317 LoC) into four topic files — `app.go`, `app_jobs.go`, `app_backup.go`, `app_credentials.go` — with zero API or behavior changes.
- Added unit-test coverage for `Provision()` and `ImportBackup()` happy paths and a representative failure per flow.

### Frontend structure and type safety
- Split `web/src/pages/Provision.svelte` (1336 LoC) into per-section sub-components (Sys, MQTT, WS, BLE, Cloud, Matter, Wifi, OTA), each owning its own form state. The JSON editor and credential reference remain peers.
- Tightened `web/src/lib/api.ts` payloads from `unknown`/`object` to named interfaces (`BulkActionRequest`, `ProvisionResult`, `FirmwareUpdateResult`, …) that mirror the Go structs.
- Introduced a Vitest + jsdom harness under `web/` with smoke tests on the API client and provision state helpers (19 tests). CI now runs `npm test` before the build.

### Frontend UX, accessibility, and resilience
- Accessibility pass: added `aria-label` to icon-only buttons (sort indicators, row actions), wrapped decorative glyphs in `aria-hidden="true"`, added `role="alert"`/`role="status"` + `aria-live` regions to error and status panels, and populated `aria-valuenow/min/max` + `aria-busy` on progress bars.
- Consolidated 26 duplicated sortable `<th>` blocks in `Devices.svelte` into a reusable `<SortHeader>` component.
- Added transient-network retry/backoff to the API client — 2 retries with 200/400 ms backoff on idempotent methods only. Mutations and HTTP status errors are never retried.
- Made Vite minification settings explicit (`minify: 'esbuild'`, `target: 'es2020'`, `cssMinify: true`) and added a CI bundle-size budget gate (`web/scripts/check-bundle-size.mjs`) that enforces raw + gzip budgets for the JS and CSS bundles.

### Docs
- Fixed the entry-point path in `CLAUDE.md` (now `cmd/shellyctl/main.go`).
- Documented the new services-file layout in `docs/ARCHITECTURE.md` and the new test/bundle-budget commands in `docs/DEVELOPMENT.md` and `CONTRIBUTING.md`.

## [0.0.8] - 2026-04-16

- Drop Gen1 device support: all HTTP REST (GET-based) Shelly code paths removed from scanner, provisioner, setters, compliance, and frontend. Devices with unknown generation now default to Gen2. Templates containing `gen1_http` sections are gracefully skipped rather than applied.

## [0.0.7] - 2026-04-16

- Fix `RandomSecret()` to panic instead of silently returning a hardcoded fallback when `crypto/rand` is unavailable
- Upgrade `golang.org/x/crypto` to v0.45.0 and `golang.org/x/net` to v0.47.0 (resolves all 5 Dependabot alerts)
- Add CI workflow: `go test ./...` and frontend build run on every push and PR to main
- Add tests for `isProvisionTargetAllowed()` covering all address categories
- Bump frontend package version to match release

## [0.0.6] - 2026-04-16

- Fixed lat/lon values being silently dropped when saving provisioning templates (inputs now use `type=number`)
- Added Delete and Rename template actions directly on the Provision page
- Removed redundant Templates section from Settings (managed on Provision page)
- Aligned section order between Provision and Compliance pages (both now lead with sys)
- Aligned sys field order between pages (lat/lon after RPC UDP Port on both)
- Extended provisioner, scanner, compliance, and setter internals

## [0.0.5] - 2026-04-15

- Public repo readiness work: root security/contributing docs, issue templates, changelog, and local-artifact ignore rules
- Added per-device detail and API docs pages to the embedded UI
- Standardized `Last Success` time presentation across Devices and per-device detail
- Expanded the documented OpenAPI v1 route surface and tightened missing-asset handling

## [0.0.4] - 2026-04-14

- Added configurable refresh timeout handling and stale-device signaling in the Devices view
- Clarified successful refresh timing with `Last Success` wording
- Added database migration support for device refresh-state tracking
- Published a GitHub Actions workflow for GHCR image releases
- Aligned Docker Compose defaults with `ghcr.io/buliwyf42/shellyadmin`

## [0.0.3] - 2026-04-08

- Added delete-all log cleanup in the API and Logs page
- Improved device table UX with auto-refresh, clearer row actions, and more visible IP links
- Added About page version and commit visibility
- Hardened authentication, API mutation handling, and job concurrency behavior
- Expanded docs for deployment, refresh behavior, and architecture alignment
