# ADR-0010: Per-Device Action Discovery via `Shelly.ListMethods`

- Status: `Accepted`
- Date: 2026-05-07
- Implements: v0.1.8 first wave (catalog + 4 fleet-wide actions);
  follow-up wave adds per-component fan-out and `ota_revert` per the
  rollout plan below.
- Roadmap link: [docs/roadmap.md](../roadmap.md) → "Broader action discovery"

## Context

The per-device action list is hard-coded in
`internal/services/device_surface.go:describeDeviceActions()`. As of v0.1.7
the surface is fixed at five actions for every device, regardless of model:

```
refresh | firmware_check | firmware_update | reboot | ble_pair
```

`ble_pair` already does runtime feature gating — it returns `skipped` on
firmware that lacks `BLE.Pair` instead of failing. That works for one
optional method but doesn't scale: the more model-specific actions we add
(component toggles, factory reset, script lifecycle, cover commands), the
more "click → silently no-ops on this model" surprises operators get.

The roadmap's framing was right: *survey per-component RPC availability
before exposing it in the UI*. The Shelly Gen2+ JSON-RPC API gives us a
direct channel for this — `Shelly.ListMethods` returns the device's exact
RPC surface (we already used it in v0.1.6 to reverse-engineer auto-update).
By caching that list per device and gating the action catalog on it, the UI
can stop showing unsupported actions altogether instead of relying on
runtime "skipped" responses.

## Goals

1. Add a curated catalog of new device actions (component toggle, cover
   open/close/stop, factory reset, script start/stop, wifi rescan, etc.)
   without shipping any action that's not actually supported by the
   specific device the operator is looking at.
2. Make the discovery mechanism reusable: future actions land in a
   declarative registry, not a hand-edited switch in `ExecuteDeviceAction`.
3. Stay safe: high-risk actions (factory reset, ota revert, mass reboot
   from the bulk surface) keep the existing confirmation + audit guarantees
   from ADR-0002.
4. Keep the action list interactive — no full firmware check round-trip
   required just to know what a device can do.

## Non-goals

- Plug-in actions or operator-defined actions. The catalog stays
  curated in Go; operators get what we ship.
- Remote-only actions that require Shelly Cloud. Catalog is RPC-only.
- Touching scheduling / automation. Out of scope.
- Bulk-action expansion beyond the existing set
  (`reboot`, `set_*`, `set_auto_update`). We're focused on per-device
  actions; a bulk-action expansion is its own follow-up.

## Decision (high level)

1. **Per-device cached method list.** Add a `supported_methods` TEXT
   column on the `devices` row holding a JSON array of method names from
   `Shelly.ListMethods`. Populated on every scan, refresh, and
   firmware-check (one extra cheap RPC per device — methods are returned
   in a single shot, ~2-4 KB unmarshalled).
2. **Declarative action registry.** Replace the hard-coded slice in
   `describeDeviceActions` with a top-level `actionCatalog []ActionDef`.
   Each entry declares the methods it requires plus an `Apply` function
   pointer. Discovery becomes a filter, not a switch.
3. **Method gate, not "skipped" runtime fallback.** Actions whose
   required methods aren't in `supported_methods` are omitted from
   `ListDeviceActions` rather than rendered with `Supported: false`.
   Preserves the "what you see is what you get" contract.
4. **Component-aware actions.** Some actions need to know *which*
   instance to act on (e.g. `switch:0` vs `switch:1`). The catalog gains
   a `componentDiscovery` field that, when set, expands one catalog
   entry into N actions at runtime — one per component instance the
   device reports.

## Action catalog (proposed first iteration)

| ID | Label | Required methods | Risk | Notes |
|----|-------|------------------|------|-------|
| `refresh` | Refresh | (none — local-only) | low | unchanged |
| `firmware_check` | Firmware Check | `Shelly.CheckForUpdate` | low | unchanged |
| `firmware_update` | Firmware Update | `Shelly.Update` | high | unchanged |
| `reboot` | Reboot | `Shelly.Reboot` | low | unchanged |
| `ble_pair` | BLE Pair | `BLE.Pair` | low | gating moves from runtime to catalog |
| `factory_reset` | Factory Reset | `Shelly.FactoryReset` | **high** | confirm-typed-name dialog |
| `factory_reset_to_initial` | Reset Wi-Fi & Cloud | `Shelly.ResetWiFiConfig` | high | softer reset variant |
| `wifi_scan` | Scan Wi-Fi | `Wifi.Scan` | low | returns visible SSIDs in detail panel |
| `eth_test` | Ethernet Status | `Eth.GetStatus` | low | refresh single-component status |
| `cover_open:N` | Cover N — Open | `Cover.Open` + component `cover:N` | medium | per-instance |
| `cover_close:N` | Cover N — Close | `Cover.Close` + component `cover:N` | medium | per-instance |
| `cover_stop:N` | Cover N — Stop | `Cover.Stop` + component `cover:N` | low | per-instance |
| `switch_toggle:N` | Switch N — Toggle | `Switch.Toggle` + component `switch:N` | medium | per-instance |
| `light_toggle:N` | Light N — Toggle | `Light.Toggle` + component `light:N` | low | per-instance |
| `script_start:ID` | Script ID — Start | `Script.Start` + component `script:ID` | low | per-instance |
| `script_stop:ID` | Script ID — Stop | `Script.Stop` + component `script:ID` | low | per-instance |
| `ota_revert` | Roll Back Firmware | `OTA.Revert` | **high** | requires `restart_required` flow afterward |

Note on `factory_reset`: the existing safety pattern (single click
confirm) is not enough for `Shelly.FactoryReset` — it bricks the
provisioning state. Per ADR-0002 we keep "click confirmation is
sufficient" as the default, but this single action gets a typed-name
prompt because the failure mode is unrecoverable from the app side.
This is a deliberate ADR-0002 carve-out, not a policy reversal — the
ADR talks about "operator intent remains explicit and auditable" and the
typed prompt strengthens both.

## Backend changes

### 1. Storage

`internal/db/migrations/021_device_supported_methods.sql`:

```sql
ALTER TABLE devices ADD COLUMN supported_methods TEXT NOT NULL DEFAULT '';
```

`internal/models/device.go`:

```go
SupportedMethods []string `json:"supported_methods"`
```

…persisted as JSON in the column. Reading: `json.Unmarshal` on Scan;
writing: `json.Marshal` on UpsertDevice. Empty string = not yet probed
(treat as "unknown set"; fall back to today's hardcoded surface so a
device still works during the discovery rollout window).

### 2. Method probe

A small `internal/core/shellyclient` helper plus a thin wrapper:

```go
// internal/core/firmware/methods.go (or a new internal/core/methods package)
func ListSupportedMethods(ctx context.Context, ip string, gen int, opts Options) ([]string, error) {
    if gen < 2 { return nil, errors.New("gen1 not supported") }
    payload, err := client.RPC(ctx, ip, "Shelly.ListMethods", nil)
    if err != nil { return nil, err }
    raw, _ := payload["methods"].([]any)
    out := make([]string, 0, len(raw))
    for _, m := range raw {
        if s, ok := m.(string); ok && s != "" { out = append(out, s) }
    }
    sort.Strings(out)
    return out, nil
}
```

Best-effort callers:
- `runFirmwareJob` and `device_surface.firmware_check` action — already
  doing two RPCs per device, this is the third (cheap).
- `runRefreshJob` and `RefreshDevice` — already doing CheckForUpdate +
  Schedule.List per device, add this fourth.
- New per-device `Refresh Methods` action — for operators who need to
  re-probe between firmware versions without doing a full refresh.

`refreshFirmwareCache` becomes `refreshDeviceCapabilities` and now
populates both the firmware cache and `SupportedMethods`.

### 3. Action registry

Replace the inline slice in `describeDeviceActions` with:

```go
type ActionDef struct {
    ID              string
    Label           string
    Description     string
    Risk            string             // "low" | "medium" | "high"
    RequiresOnline  bool
    RequiresAuth    bool               // for actions that bypass the
                                       // refresh's auth-required short
                                       // circuit
    RequiredMethods []string           // ALL must be in SupportedMethods
    Component       string             // empty for fleet-wide actions;
                                       // "switch" / "cover" / "light" /
                                       // "script" for per-component fan-out
    Apply           func(context.Context, *AppService, models.Device, DeviceActionRequest) (DeviceActionResult, error)
}

var actionCatalog = []ActionDef{ /* … */ }
```

`ListDeviceActions` becomes:

```go
func describeDeviceActions(device models.Device) []DeviceAction {
    methods := methodSet(device)
    var out []DeviceAction
    for _, def := range actionCatalog {
        if !methodsCovered(methods, def.RequiredMethods) { continue }
        if def.Component == "" {
            out = append(out, materialize(def, device))
            continue
        }
        for _, inst := range componentInstances(device, def.Component) {
            out = append(out, materialize(def, device, withInstance(inst)))
        }
    }
    return out
}
```

`ExecuteDeviceAction` becomes a single dispatch by ID + invocation of
`def.Apply`, replacing the giant switch.

### 4. Component instance discovery

Component instances live in `Shelly.GetStatus` keys (`switch:0`,
`cover:1`, `light:0`, `script:1`, etc.). We already store the full
status JSON in `Device.RawStatus`; the helper reads keys matching
`<componentType>:N` and returns `[]int` of N values. Handles the case
where N is not contiguous (skipped IDs).

For the script case, `Script.List` is preferred since scripts can be
configured but not currently in status. Fall back to status-key parse
if `Script.List` isn't supported.

### 5. Audit log

Each `Apply` returns a `DeviceActionResult` that already flows through
`s.LogCtx`. The catalog gains an `auditMessage` template per action
(e.g. `factory_reset: "device action factory_reset target=%s"`). Risk
level is included in every audit line so a future "show all high-risk
actions in the last 7 days" filter has structured input.

## Frontend changes

### Per-device detail page

The action list already renders dynamically from
`/api/devices/:target/actions`. Two minor adjustments:

- **Group by risk**, not insertion order. Low-risk actions (refresh,
  status reads) at the top; medium (component control) below; high
  (factory reset, OTA revert) bottom in a visually-separated panel.
- **Per-component badges** so operators see e.g. "Switch 0 — Toggle"
  and "Switch 1 — Toggle" as distinct rows.
- **Typed-name confirm modal** for actions with `Risk: "high"`. Reuses
  the modal we already built for the firmware-install confirm; second
  variant requires the operator to type the device's `name` exactly.

### Devices page

No changes for the discovery itself. Optional follow-up: a new column
"Capabilities" with badges for cover/switch/light/script counts so
operators can spot at-a-glance which devices support which actions.
Defer to a separate plan.

## Testing

Unit tests on the registry:

- `methodsCovered` — empty `RequiredMethods` always covered;
  partial coverage rejected; missing methods rejected.
- `componentInstances` — happy path (`switch:0`, `switch:1`),
  non-contiguous IDs, no instances.
- `describeDeviceActions` — deterministic action ordering across runs;
  high-risk actions don't appear without their methods even on a Pro 4
  PM (which exposes most of them).

Integration tests that hit a real device (only run when
`SHELLYADMIN_TEST_DEVICE_IP` is set):

- `ListSupportedMethods` round-trip.
- `Wifi.Scan` action returns >0 SSIDs on a non-isolated test bench.
- `factory_reset` is **excluded** from the integration suite by design.

Frontend Vitest:

- The action-list renderer groups correctly by risk.
- The typed-name modal blocks submission until input matches.

## Risks and mitigations

1. **`Shelly.ListMethods` is missing on very old firmware.** Treat as
   "no information" — fall back to the v0.1.7 hardcoded action set so
   the UI never goes empty. Logged once per device per upgrade window.
2. **`Apply` functions diverge from the existing `ExecuteDeviceAction`
   error contract.** Mitigated by adding a thin shim in the catalog
   itself: every `Apply` returns `DeviceActionResult` and any error is
   converted to a `Status: "failed"` with the friendly RPC error text
   from `friendlyRPCError` (introduced in v0.1.5).
3. **High-risk action list grows over time.** The typed-name confirm
   stops being a useful gate if operators are habituated to it. Build
   into the design from day one: only `factory_reset`, `ota_revert`,
   and any future actions that produce *unrecoverable* state require
   the typed prompt. `firmware_update` and `reboot` keep the existing
   click-only confirm.
4. **Per-component fan-out clutters the action list on
   high-component-count devices** (a Pro 4 PM has 4 switches → 4 toggle
   actions). Mitigation: collapse same-type actions under an expander
   when count > 2.
5. **SQLite column migration risk** — the new column is nullable with
   default `''`, so the migration is forward-only and zero-downtime.

## Rollout plan

Single release, no flag gating needed because the new actions only
*appear* when the device's method list claims support. Sequence:

1. Migration 021 (column add).
2. Backend: probe + storage + catalog refactor + first wave of new
   actions (factory_reset, wifi_scan, eth_test, cover_*).
3. Frontend: risk-grouped action list + typed-name modal.
4. Per-component fan-out (switch_toggle, light_toggle, script_*) —
   second wave, separate PR once the catalog plumbing is settled.

## Open questions

- Should `supported_methods` be exposed in the per-device detail page's
  raw-status panel for transparency? Probably yes — same panel that
  already shows raw config / status.
- Do we want a "device capability profile" abstraction (model →
  expected method set) so we can flag *unexpected* missing methods
  ("Pro 4 PM should have Switch.Toggle but doesn't")? Probably *not*
  for v1 — adds a model-database maintenance burden without immediate
  value. Revisit if operators ask.
- What happens when a device's method set changes mid-flight (e.g.
  firmware update adds new methods)? The next firmware-check or
  refresh re-probes; the action list re-renders next time the operator
  opens the device detail page. Acceptable latency.
