# ADR-0009: Firmware Auto-Update via `Schedule.*` Synthesis

- Status: `Accepted`
- Date: 2026-05-06

## Context

Shelly Gen2+ devices expose an "Enable auto update firmware" toggle in their
local web UI (added in firmware 1.2.0), but the JSON-RPC API has **no
dedicated method** for reading or writing this setting. We verified this
directly:

- `Sys.GetConfig` does not contain an `autoupdate` field. The full top-level
  set is `device | location | debug | ui_data | rpc_udp | sntp | cfg_rev`.
- `Shelly.GetConfig` contains no `ota` block. There is no `OTA.GetConfig` /
  `OTA.SetConfig` / `Sys.SetAutoUpdate` / `AutoUpdate.SetConfig` method —
  every candidate returns `404 No handler for ...`. The `OTA.*` methods that
  exist (`OTA.Start/Write/Data/Abort/Commit/Revert`) are byte-level
  chunked-upload plumbing, not configuration.
- The official Shelly Gen2 docs (Sys component, Shelly component, changelog)
  confirm no public RPC for auto-update settings.

ShellyAdmin's previous attempt at auto-update support (`OTAAutoUpdate`
compliance field, `ota` provisioner section) was removed in v0.0.14 / v0.0.16
because there was nothing on the device to read or set.

Reverse-engineering the device's local web UI bundle revealed the actual
mechanism: when the user toggles auto-update in the Shelly UI, the device
firmware **synthesises a `Schedule.*` job** that calls `Shelly.Update` on a
recurring timer. The marker is `calls[0].origin = "shelly_service"`. A live
`Schedule.List` against a device with auto-update on stable returns:

```json
{
  "id": 1,
  "enable": true,
  "timespec": "0 0 0 * * 0,1,2,3,4,5,6",
  "calls": [{
    "method": "Shelly.Update",
    "params": {"stage": "stable"},
    "origin": "shelly_service"
  }]
}
```

This is the same surface ShellyAdmin already uses for one-shot updates
(`Shelly.Update`) and the same `Schedule.*` API used for any user-defined
scheduled job — so we can read and write auto-update without a new
device-side capability.

## Decision

ShellyAdmin treats the auto-update setting as a **synthesised Schedule
entry**, not a config field. Specifically:

1. **Read**: `Schedule.List` is filtered for jobs where any
   `calls[].method` equals `Shelly.Update` (case-insensitive) AND
   `calls[].origin` equals `"shelly_service"`. The first matching, enabled
   job's `params.stage` value (`stable` / `beta`) is the device's
   auto-update mode. No matching enabled job → `off`.

2. **Write**:
   - `off` → delete every existing `origin == "shelly_service"`,
     `Shelly.Update` Schedule entry via `Schedule.Delete`.
   - `stable` / `beta` → first delete any existing entries (idempotent),
     then `Schedule.Create` a single new entry with
     `enable: true`, `timespec: "0 0 0 * * 0,1,2,3,4,5,6"` (cron-style:
     daily at midnight),
     `calls: [{method: "Shelly.Update", params: {stage: <value>},
                origin: "shelly_service"}]`.

3. **Origin marker is load-bearing.** User-created `Schedule.*` jobs that
   happen to call `Shelly.Update` (with any other origin, or no origin) are
   **not** modified by ShellyAdmin's auto-update operations. Conversely, the
   `shelly_service` marker is what the device's own web UI writes; preserving
   it means our writes round-trip cleanly through the device UI and vice
   versa.

4. **Persistence**: the resolved mode is stored on the `devices` row as
   `fw_auto_update` (`""` = never read | `off` | `stable` | `beta`), refreshed
   on every firmware check and every Refresh. Migration
   `018_device_fw_auto_update.sql`.

5. **Surface**: read state appears as a column on the Firmware and Devices
   pages; bulk write is exposed via the Firmware page's
   `Auto → Off / Stable / Beta` buttons (action `set_auto_update`); a
   compliance rule (`auto_update_stage`) flags non-conformant devices; the
   provisioner accepts a top-level `auto_update` template section.

## Consequences

- Auto-update becomes a fleet-configurable setting despite the device
  firmware not exposing a dedicated method.
- The implementation is fragile to one specific Shelly behaviour: if a
  future firmware changes the marker (`origin: "shelly_service"`),
  ShellyAdmin will start showing `off` for devices that are actually
  scheduled. This is an acceptable risk because the marker is what the
  device's own web UI uses — changing it would also break the device's
  built-in toggle UI.
- The compliance rule is automatically skipped on devices that haven't been
  firmware-checked yet (`fw_auto_update == ""`), so mixed fleets with
  pre-firmware-check devices don't false-positive.
- Operators who manually create `Shelly.Update` Schedule entries with a
  different `origin` value get a guaranteed no-clobber path: ShellyAdmin's
  bulk auto-update controls won't touch them.
- The schedule itself (daily at 00:00) is hard-coded to match what the
  device UI writes. Configurable timespec is a future ADR if needed; for
  now matching the built-in UI minimises behaviour drift.
