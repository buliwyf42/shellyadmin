# Phase 4b — Services Split + Frontend Refactor (M2, M6, M7, M8)

> **Status**: planned. Targeted release: v0.3.0-rc (or v0.3.0 if no pre-release).
> **Aggregate aufwand**: 5–10 working sessions across 4 sub-blocks.
> **Risk**: each block touches a wide test surface; ship as 4 separate PRs.

This sub-plan covers the four XL items deferred from Phase 3 + Phase 4a.
Together they're the architectural and ergonomic restructure that v0.3
hangs on. Each block is independently shippable: M7 first (clears the
service-layer touch radius), M8 next (relies on M3 codegen evolution),
then M2 (frontend page split), finally M6 (CSP without `unsafe-inline`
— gated on M2 because Svelte component styles compile to inline `<style>`).

---

## Block 4b.1 — M7: services internal split

**Aufwand**: 2–3 sessions. **Risk**: high (50+ methods on `AppService`).

### Why

`internal/services/app.go` is ~1000 LOC; `app_jobs.go` is ~1000 LOC;
together with the per-resource files (app_mcp.go, app_credentials.go,
app_export.go, etc.) `services` is the project's single largest
package. AppService has 50+ methods crossing concerns
(job lifecycle, MCP listener, credential CRUD, audit logging, backup,
session validation). Every new feature touches `AppService` directly;
test fixtures (fakes_test.go) have to implement every method whether
the test cares or not.

### Target shape

```
internal/services/
  app.go                  # lifecycle + Stop + Start + dependency wiring
  store.go                # Store interface (kept here; the seam)
  password.go             # unchanged
  sanitize_log.go         # unchanged
  audit_webhook.go        # unchanged (already split)
  sessions/
    sessions.go           # IssueSession/RevokeSession/Validator (moved)
    sessions_test.go
  jobs/
    refresh.go            # RefreshDevices, runRefreshJob
    firmware_check.go     # StartFirmwareCheck + runFirmwareJob + scheduler
    firmware_install.go   # StartFirmwareInstall + runFirmwareInstall + poll
    scan.go               # StartScan, ScanStatus, ConfirmScan + worker
    scheduler.go          # runFirmwareCheckScheduler + recover guard
    retention.go          # auditRetentionLoop + runAuditRetentionOnce
    backup.go             # autoBackupLoop + runAutoBackupOnce
  mcp/
    controller.go         # MCPController (moved from app_mcp.go)
  credentials/
    crud.go               # SaveCredential, DeleteCredential, etc.
    groups.go             # group + assignments
  backup/
    export.go             # ExportBackup
    import.go             # ImportBackup
```

### Approach (low-risk, incremental)

1. **Move first, refactor second.** Each sub-package's first commit is
   a pure-move with `// MOVED FROM internal/services/X` comments at
   the top. No signature changes. The package boundary is established;
   public surface stays via re-exported funcs on `*AppService`.
2. **AppService.{sessions, jobs, mcp, credentials, backup} fields** —
   the lifecycle owner keeps a reference to each sub-service; method
   delegation looks like `s.jobs.RefreshDevices(ctx)`. Existing
   call sites stay compiling because `AppService.RefreshDevices` is
   a one-liner that delegates.
3. **Cross-cutting deps** (`logf`, `ctx`, `metrics`, `Store`) are
   passed to each sub-service at construction time as a small `Deps`
   struct, not a re-export of `AppService`.
4. **Tests follow** — `fakes_test.go` shrinks to just the methods each
   sub-service uses; existing service-level tests migrate to the
   sub-package they belong in.

### Acceptance

- `internal/services/app.go` < 200 LOC (lifecycle + wiring only).
- `internal/services/jobs/` is the new home for every `run*Job`
  function; `app_jobs.go` deleted.
- `internal/services/fakes_test.go` deleted; each sub-package has its
  own lightweight fake.
- `go test ./...` green; existing test count + coverage equal or better.
- One commit per sub-block (`refactor(services/sessions)`,
  `refactor(services/jobs)`, etc.) — easier to revert if a single move
  breaks.

### Risks + mitigations

| Risk | Mitigation |
|---|---|
| Method removal from `AppService` breaks an unseen caller | Keep delegators on `AppService` for one release cycle; mark removal as `v0.4`. |
| Cyclic deps if `jobs` needs `mcp` for some lifecycle ordering | Keep MCP shutdown in `app.go` Stop(); `jobs` only sees `Store + clock + logf`. |
| Tests have to re-mount fakes per-sub-package | Acceptable; per-package fakes are smaller + easier to maintain than the current 30-method monolith. |

### Out of scope

- Renaming methods (`StartFirmwareInstall` stays; `firmware_install.Start` is the package-internal name).
- Changing the `Store` interface partition — that's a separate refactor.

---

## Block 4b.2 — M8: `models.Device` split into `DeviceListView` + `DeviceDetail`

**Aufwand**: 1–2 sessions. **Risk**: medium (touches DB row scan + handlers).

### Why

`models.Device` carries 56 fields. The Devices page only needs ~20 of
them for the table view; the Device Detail page needs all 56 plus
`raw_config` / `raw_status` snapshots (which can be tens of KB each).
Today the SPA fetches the full payload for the list view, paying the
serialisation + transmit cost on every refresh tick.

### Target shape

```go
// internal/models/device.go
type DeviceListView struct {
    MAC, IP, Name, Model, App, FW string
    Gen int
    Online bool
    LastSeen, FirstSeen string
    Compliant bool
    ComplianceIssues []string
    // ... ~20 fields total
}

type DeviceDetail struct {
    DeviceListView
    // The rest of today's models.Device:
    RawConfig, RawStatus string
    Lat, Lon *float64
    // ... ~36 more fields
}
```

### Approach

1. Move the 20 list-view fields into `DeviceListView`. Keep
   `DeviceDetail` as the embedded superset; existing code that uses
   `models.Device` is renamed to `models.DeviceDetail` via mechanical
   `sed`.
2. New `DB.ListDevicesView() []DeviceListView` method that scans only
   the slim columns; existing `DB.ListDevices()` (full) renames to
   `ListDevicesDetail()`.
3. `handler_devices.go` GetDevices switches to the slim view;
   GetDeviceDetail keeps the full payload.
4. Frontend: `Device` interface in `types.ts` splits matching the Go
   side; M3 schema check picks up the drift gate.
5. Re-measure the `/api/devices` response size before/after; expect
   30-50% reduction on a 50-device fleet.

### Acceptance

- `/api/devices` payload size halved (measured against a fixture).
- Devices page render time unchanged (sub-100ms).
- M3 schema check passes after the model split.
- No breaking change for MCP consumers (`list_devices` continues to
  return the slim shape; `get_device` returns the detail shape — the
  current behaviour, just better-typed).

### Risks + mitigations

| Risk | Mitigation |
|---|---|
| Hidden caller relies on a "detail" field from `ListDevices` | grep all call sites; add a deprecation tag on the full-fat `ListDevicesDetail` (it should only be called from `GetDeviceDetail`). |
| Compliance.Evaluate needs detail fields it currently gets free | Move compliance evaluation to the detail path; the list view shows the cached `Compliant` bool stamped at refresh-time. |

---

## Block 4b.3 — M2: Frontend page split (Compliance, Devices, Provision)

**Aufwand**: 2–3 sessions. **Risk**: high (touches every authenticated SPA page).

### Why

Three pages are >1000 LOC each:

```
web/src/pages/Compliance.svelte   1141 LOC
web/src/pages/Devices.svelte      1098 LOC
web/src/pages/Provision.svelte    1046 LOC
```

Any change to a single button means re-reading the whole file. Vitest
component tests (S16) are stuck at the "test the API path" level
because mounting a 1000-LOC Svelte component in jsdom is fragile.

### Target shape

```
web/src/pages/Compliance.svelte         (~400 LOC: layout + state)
web/src/pages/compliance/
  RuleEditor.svelte
  CustomRulesList.svelte
  DeviceMatrix.svelte
  ComplianceActions.svelte
web/src/pages/Devices.svelte            (~400 LOC: layout + state)
web/src/pages/devices/
  DeviceTable.svelte
  DeviceRowActions.svelte
  BulkActionBar.svelte
  ColumnPicker.svelte
web/src/pages/Provision.svelte          (~400 LOC: layout + state)
web/src/pages/provision/  (already exists for section forms)
  TemplatesPanel.svelte
  IPListPanel.svelte
  ResultsPanel.svelte
```

### Approach

1. **Identify mount-points.** For each big page, locate the
   sub-component cluster boundaries by reading the template
   block-by-block. Most pages have a clear "left column / right
   column / modal" tripartition.
2. **Extract one section at a time.** Start with the most isolated
   (typically the modal). The pattern is: cut the template +
   relevant `<script>` state into a child component, prop the state
   in, emit events back up.
3. **Vitest covers each extracted child** — that's S16's deferred
   half: now that the components are small, a `mount(Child, { props })`
   test runs in <100ms.
4. **Visual regression** — manual; the SPA's bundle-size budget
   (`npm run check:bundle-size`) catches accidental balloon.

### Acceptance

- Each top-level page < 500 LOC.
- Bundle size delta < 10% (extracts should not duplicate logic).
- Vitest count grows from 57 to ~80 (one happy-path test per new
  component).
- Bundle-size budget passes.

### Risks + mitigations

| Risk | Mitigation |
|---|---|
| Reactive state passes through too many props | Use Svelte stores for cross-component state, not prop-drilling. |
| Modal extraction changes focus-trap behaviour | Add a focus-management test for each modal child. |
| Hot-reload time regresses | Measure before/after; ship a sub-block per page so regressions are isolated. |

---

## Block 4b.4 — M6: CSP without `unsafe-inline`

**Aufwand**: 1–2 sessions. **Risk**: medium (CSP failures break the SPA silently).

**Depends on M2** (some inline styles only collapse when the per-page
extracts ship).

### Why

`Content-Security-Policy: style-src 'self' 'unsafe-inline'` is the
last remaining concession to inline content. Removing it lets a future
DOM-injection sink fail closed (the browser rejects the inline style
before the injected script can paint anything).

### Approach

1. **Audit inline styles.** `vite build` → `dist/index.html` →
   `grep -E "<style|style=\""`. Svelte components compile to
   `<style>` blocks in the bundled HTML; per-component style hashes
   land in CSS bundles.
2. **Move every `style="..."` attr into a class.** The handful that
   remain (often progress bars, dynamic widths) accept a CSS custom
   property + a static class.
3. **Bundle the remaining `<style>` blocks** via
   `vite-plugin-singlefile` or Svelte's `compilerOptions.css =
   "external"` — emit a single hashed CSS file the SPA loads via a
   `<link rel="stylesheet">`.
4. **Update `internal/middleware/security.go`** — drop
   `'unsafe-inline'` from `style-src`; ship behind a `style-src-hash`
   directive if any hashed inline blocks remain.
5. **Verify in Chrome + Firefox + Safari** that the SPA renders
   identically. Chrome DevTools "Console" panel reports CSP
   violations clearly; absence of console errors after a full UI
   tour is the acceptance signal.

### Acceptance

- `Content-Security-Policy: style-src 'self'` (no `unsafe-inline`).
- No browser-console CSP violations on a full UI tour
  (Login → Devices → Provision → Settings → Logs → Compliance).
- Bundle size delta < 5%.

### Risks + mitigations

| Risk | Mitigation |
|---|---|
| CSP violation only fires on a rarely-used UI path | The full UI tour acceptance criterion catches this; pair with E2E playwright (T8) which is queued for v0.3. |
| Svelte runtime style-injection breaks on a future Svelte upgrade | Pin Svelte minor; treat the CSP gate as a regression suite. |

---

## Cross-block sequencing

```
M7 (services split)
  └── (unblocks any future refactor)
M8 (Device split)
  └── M2 needs the slim list view to test page-render perf
M2 (frontend page split)
  └── M6 needs M2's smaller pages to feasibly remove inline styles
M6 (CSP unsafe-inline)
```

Each block is its own PR + release-candidate. After all four ship,
v0.3.0-rc can cut; v0.3.0 ships after operator soak time.
