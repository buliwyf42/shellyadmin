<!--
  Left-column device selector + precheck summary for the Provision page.
  Renders two cards: the precheck summary (only when at least one device
  is selected) and the Select Devices table with row-level checkboxes.

  Extracted from Provision.svelte in v0.3.0 (M2 — Block 4b.3 of
  docs/plans/phase-4b-refactor-block.md). The precheck `$:` derivations
  themselves stay on the parent because they depend on the
  template-builder state; we receive the computed results as props.
-->
<script lang="ts">
  import type { Device } from '../../lib/types';
  import type { SvelteSet } from 'svelte/reactivity';

  export type PrecheckIssue = {
    ip: string;
    label: string;
    reason: string;
    category: 'auth' | 'other';
  };

  export let devices: Device[];
  export let selected: SvelteSet<string>;
  export let loading: boolean;
  export let precheckEligibleCount: number;
  export let precheckIssues: PrecheckIssue[];
  export let precheckReasonCounts: { auth: number; generation: number };
  export let precheckTemplateError: string;
  export let copiedSkipped: boolean;
  export let onToggle: (mac: string, checked: boolean) => void;
  export let onSelectAll: () => void;
  export let onSelectNone: () => void;
  export let onSelectOnlyEligible: () => void;
  export let onCopySkippedIPs: () => void;

  function reasonBadgeClass(category: PrecheckIssue['category']): string {
    switch (category) {
      case 'auth':
        return 'bg-warning text-dark';
      default:
        return 'bg-secondary';
    }
  }

  function reasonBadgeText(category: PrecheckIssue['category']): string {
    switch (category) {
      case 'auth':
        return 'auth';
      default:
        return 'other';
    }
  }
</script>

{#if selected.size > 0}
  <div class="card bg-dark border-secondary">
    <div class="card-body">
      <h2 class="h6">Precheck Summary</h2>
      {#if precheckTemplateError}
        <div class="alert alert-warning py-2 mb-2">{precheckTemplateError}</div>
      {/if}
      <p class="mb-2">
        <span class="badge bg-success me-2">{precheckEligibleCount}</span> eligible
      </p>
      <p class="mb-2">
        <span class="badge bg-warning text-dark me-2">{precheckIssues.length}</span> predicted to be skipped
      </p>
      <div class="d-flex gap-2 flex-wrap mb-2">
        <button
          class="btn btn-sm btn-outline-light"
          on:click={onSelectOnlyEligible}
          disabled={precheckIssues.length === 0 || Boolean(precheckTemplateError)}
          >Select Only Eligible</button
        >
        <button
          class="btn btn-sm btn-outline-light"
          on:click={onCopySkippedIPs}
          disabled={precheckIssues.length === 0}>Copy Skipped IPs</button
        >
        {#if copiedSkipped}
          <span class="badge bg-success">copied</span>
        {/if}
        {#if precheckReasonCounts.auth}
          <span class="badge bg-warning text-dark">auth: {precheckReasonCounts.auth}</span>
        {/if}
        {#if precheckReasonCounts.generation}
          <span class="badge bg-info text-dark">generation: {precheckReasonCounts.generation}</span>
        {/if}
      </div>
      {#if precheckIssues.length > 0}
        <div class="table-responsive">
          <table class="table table-dark table-striped table-sm mb-0">
            <thead>
              <tr><th>IP</th><th>Device</th><th>Type</th><th>Reason</th></tr>
            </thead>
            <tbody>
              {#each precheckIssues as issue (issue.ip + '|' + issue.category)}
                <tr>
                  <td>{issue.ip}</td>
                  <td>{issue.label}</td>
                  <td
                    ><span class={`badge ${reasonBadgeClass(issue.category)}`}
                      >{reasonBadgeText(issue.category)}</span
                    ></td
                  >
                  <td>{issue.reason}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  </div>
{/if}

<div class="card bg-dark border-secondary">
  <div class="card-header d-flex justify-content-between align-items-center">
    <span>Select Devices</span>
    <div class="d-flex gap-2">
      <button class="btn btn-sm btn-outline-light" on:click={onSelectAll}>All</button>
      <button class="btn btn-sm btn-outline-light" on:click={onSelectNone}>None</button>
    </div>
  </div>
  <div class="card-body p-0">
    {#if loading}
      <div class="p-2 text-secondary">Loading devices...</div>
    {:else if devices.length === 0}
      <div class="p-2 text-secondary">No devices enrolled yet.</div>
    {:else}
      <div class="table-responsive device-list-scroll">
        <table class="table table-dark table-striped align-middle table-nowrap mb-0">
          <thead>
            <tr>
              <th></th>
              <th>IP</th>
              <th>Name</th>
              <th>Gen</th>
            </tr>
          </thead>
          <tbody>
            {#each devices as device (device.mac)}
              <tr>
                <td
                  ><input
                    type="checkbox"
                    class="form-check-input"
                    checked={selected.has(device.mac)}
                    on:change={(e) =>
                      onToggle(device.mac, (e.currentTarget as HTMLInputElement).checked)}
                  /></td
                >
                <td>{device.ip}</td>
                <td>{device.name || device.serial || device.mac}</td>
                <td><span class="badge bg-success">Gen{device.gen}</span></td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
  <div class="card-footer p-2 text-secondary">{selected.size} of {devices.length} selected</div>
</div>
