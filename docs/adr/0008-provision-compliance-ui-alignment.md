# ADR-0008: Provision/Compliance UI Alignment and Template Management Consolidation

- Status: `Accepted`
- Date: 2026-04-16

## Context

Two related problems emerged as the UI matured:

1. **Template management was split across pages.** The Settings page had a basic template list with delete-only actions. The Provision page had load/save but no delete or rename. Operators had to switch pages to manage templates, and there was no way to rename a template at all.

2. **Provision and Compliance section/field ordering diverged.** The Compliance page listed sections in an arbitrary order (wifi first, sys buried near the bottom). Within the sys section, lat/lon appeared at different positions on each page. Operators switching between pages to compare or cross-reference settings had to re-orient every time.

Additionally, the lat/lon inputs on the Provision page used plain text inputs, which caused values to be silently dropped on save when the internal string-to-number conversion failed (e.g. empty string, or locale-specific decimal separators).

## Decision

**Template management consolidation:**
- All template actions (load, edit, save, delete, rename) are available directly on the Provision page toolbar.
- The Settings page no longer exposes a Templates section.
- Rename is implemented as save-under-new-name followed by delete-old-name; no dedicated backend rename endpoint is needed.

**Shared section ordering:**
- Both Provision and Compliance pages follow the same section order: `sys → mqtt → cloud → ws → ble → wifi → ota`.
- Sections that only exist on one page (e.g. `matter`, `auth`, `kvs` on Provision; `custom rules` on Compliance) are appended after the shared sections.

**Shared sys field ordering:**
- Both pages order sys fields as: `tz → sntp → time_format → debug_ws → debug_udp → rpc_udp → lat → lon → eco → discoverable`.
- Provision prepends `device_name` (not present in Compliance) before `tz`.

**Numeric lat/lon inputs:**
- Both Provision and Compliance use `type=number step=0.0001` for lat/lon inputs.
- This ensures the browser normalises the value to a canonical number regardless of locale, eliminating silent data loss on save.

## Consequences

- Template lifecycle is managed in context on the Provision page; Settings is not needed for template work.
- Operators can orient to either page using the same mental model for section and field positions.
- Lat/lon values entered in the Provision form are reliably persisted to saved templates and visible in JSON view.
- Future additions to the sys section should follow the established field order on both pages simultaneously.
