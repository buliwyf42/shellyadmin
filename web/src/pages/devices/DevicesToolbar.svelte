<!--
  Page-hero toolbar for the Devices page: filter input, refresh-interval +
  firmware-channel selects, Columns / Refresh / Reboot-All buttons.

  Extracted from Devices.svelte in v0.3.0 (M2 — Block 4b.3). State is
  bound two-ways via Svelte's bind:value so the parent doesn't have to
  manage event-callback churn for the small inputs; action buttons emit
  via simple event callbacks. The persisted refreshInterval +
  firmwareChannel stores are read/written directly so other pages that
  share the same dropdown defaults stay in sync.
-->
<script lang="ts">
  import { firmwareChannel, refreshInterval } from '../../lib/stores';

  export let filter: string;
  export let onlineCount: number;
  export let loading: boolean;
  /** Total devices in the visible (post-filter, post-sort) result set —
   * disables Reboot-All when 0. */
  export let listedCount: number;
  export let showColumns: boolean;
  export let onToggleColumns: () => void;
  export let onRefresh: () => void;
  export let onRebootAll: () => void;
</script>

<section class="page-hero">
  <div class="page-title-row">
    <span class="page-kicker">Devices</span>
    <span class="page-status">{onlineCount} online</span>
    {#if loading}
      <span class="page-status muted">Refreshing…</span>
    {/if}
  </div>
  <div class="page-hero-controls">
    <input
      class="form-control toolbar-search"
      placeholder="Filter name / IP / MAC / model"
      bind:value={filter}
    />
    <select class="form-select toolbar-select" bind:value={$refreshInterval}>
      <option value={0}>Auto refresh: Off</option>
      <option value={30000}>Auto refresh: 30 sec</option>
      <option value={60000}>Auto refresh: 1 min</option>
      <option value={300000}>Auto refresh: 5 min</option>
    </select>
    <select
      class="form-select toolbar-select"
      bind:value={$firmwareChannel}
      title="Which channel's update version to highlight in the FW column"
    >
      <option value="stable">FW channel: Stable</option>
      <option value="beta">FW channel: Beta</option>
    </select>
    <button class="btn btn-outline-light" on:click={onToggleColumns}
      >{showColumns ? 'Hide Columns' : 'Columns'}</button
    >
    <button class="btn btn-warning text-dark" on:click={onRefresh} disabled={loading}
      >Refresh</button
    >
    <button
      class="btn btn-outline-warning"
      on:click={onRebootAll}
      disabled={loading || listedCount === 0}
      title="Reboot all listed devices">Reboot All</button
    >
  </div>
</section>

<style>
  .page-hero {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 0.5rem;
    padding-bottom: 0.45rem;
    border-bottom: 1px solid rgba(160, 177, 190, 0.18);
  }

  .page-title-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: nowrap;
    min-width: 0;
  }

  .page-kicker {
    font-size: 0.95rem;
    font-weight: 700;
    line-height: 1;
    white-space: nowrap;
  }

  .page-status {
    color: #39c37c;
    font-size: 0.68rem;
    font-weight: 700;
    white-space: nowrap;
  }

  .page-status.muted {
    color: #d2b14e;
  }

  .page-hero-controls {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex-wrap: nowrap;
    justify-content: flex-end;
    flex: 1 1 auto;
    min-width: 0;
  }

  .toolbar-search {
    width: var(--toolbar-control-width-lg);
    flex: 0 0 var(--toolbar-control-width-lg);
  }

  .toolbar-select {
    width: 18rem;
    min-width: 18rem;
    flex: 0 0 18rem;
  }

  .page-hero-controls :global(.form-control),
  .page-hero-controls :global(.form-select),
  .page-hero-controls :global(.btn) {
    min-height: var(--control-height-sm);
    font-size: 0.76rem;
  }

  .page-hero-controls :global(.form-select) {
    padding-right: 2rem;
  }

  .page-hero-controls :global(.btn) {
    padding-left: 0.62rem;
    padding-right: 0.62rem;
    white-space: nowrap;
  }

  @media (max-width: 900px) {
    .page-hero {
      flex-direction: column;
      align-items: stretch;
    }

    .page-hero-controls {
      width: 100%;
      justify-content: flex-start;
      flex-wrap: wrap;
    }

    .page-title-row {
      flex-wrap: wrap;
    }

    .toolbar-search {
      width: 100%;
      flex-basis: 100%;
    }
  }
</style>
