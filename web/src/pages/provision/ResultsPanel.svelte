<!--
  Post-provision results panel for the Provision page: per-device
  section-status badges + a "Reboot the N devices that require restart"
  button for the subset whose template applied a Wifi/Eth/Sys change
  that needs a power-cycle to take effect.

  Extracted from Provision.svelte in v0.3.0 (M2 — Block 4b.3 of
  docs/plans/phase-4b-refactor-block.md). Self-contained: the panel
  receives the results + devices list as props and emits a single
  onError callback for failures inside the inline Reboot button (the
  rest is read-only display).
-->
<script lang="ts">
  import { api } from '../../lib/api';
  import type { Device, ProvisionResult } from '../../lib/types';

  export let results: ProvisionResult[];
  export let devices: Device[];
  export let running: boolean;
  /** Callback fired when the inline Reboot button's bulk action errors.
   * Parent surfaces the failure via its ErrorNotice. */
  export let onError: (err: unknown) => void;

  $: restartRequiredDevices = results.filter((r) => r.restart_required);
</script>

<div class="card bg-dark border-secondary mt-3" role="status" aria-live="polite">
  <div class="card-body">
    <div class="d-flex align-items-center justify-content-between mb-3 flex-wrap gap-2">
      <h2 class="h5 mb-0">Results</h2>
      {#if restartRequiredDevices.length > 0}
        <button
          class="btn btn-sm btn-warning"
          disabled={running}
          on:click={async () => {
            const macs = restartRequiredDevices
              .map((r) => devices.find((d) => d.ip === r.info.ip)?.mac)
              .filter(Boolean) as string[];
            if (!macs.length) return;
            if (!confirm(`Reboot ${macs.length} device(s) that require a restart?`)) return;
            try {
              await api.bulk({ action: 'reboot', macs });
            } catch (err) {
              onError(err);
            }
          }}
          >Reboot {restartRequiredDevices.length} restart-required device{restartRequiredDevices.length !==
          1
            ? 's'
            : ''}</button
        >
      {/if}
    </div>
    <div class="table-responsive">
      <table class="table table-dark table-sm align-middle mb-0">
        <thead>
          <tr>
            <th>IP</th>
            <th>Device</th>
            <th>Status</th>
            <th>Sections</th>
          </tr>
        </thead>
        <tbody>
          {#each results as r (r.info.ip)}
            {@const overallOk = r.results.every((s) => s.status === 'ok' || s.status === 'skipped')}
            {@const hasFailed = r.results.some((s) => s.status === 'failed')}
            <tr>
              <td class="text-monospace small">{r.info.ip}</td>
              <td class="small">{r.info.name || r.info.model || '—'}</td>
              <td>
                {#if hasFailed}
                  <span class="badge bg-danger">failed</span>
                {:else if overallOk}
                  <span class="badge bg-success">ok</span>
                {:else}
                  <span class="badge bg-secondary">partial</span>
                {/if}
                {#if r.restart_required}
                  <span class="badge ms-1" style="background:#c89a2a;color:#fff;"
                    >restart required</span
                  >
                {/if}
              </td>
              <td>
                <div class="d-flex flex-wrap gap-1">
                  {#each r.results as s (s.section)}
                    <span
                      class="badge {s.status === 'ok'
                        ? 'bg-success'
                        : s.status === 'skipped'
                          ? 'bg-secondary'
                          : 'bg-danger'}"
                      title={s.detail}>{s.section}</span
                    >
                  {/each}
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
</div>
