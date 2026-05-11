<!--
  Column-visibility picker for the Devices page. Renders a 3-column grid
  of checkboxes (one per column declared in stores.ts:deviceColumns); each
  toggle writes through to the persisted colVis store, so the selection
  survives reloads.

  Extracted from Devices.svelte in v0.3.0 (M2 — Block 4b.3 of
  docs/plans/phase-4b-refactor-block.md). Parent renders us conditionally
  via `{#if showColumns}` so this component owns no visibility state.
-->
<script lang="ts">
  import { colVis, deviceColumns } from '../../lib/stores';

  function toggleColumn(key: string, checked: boolean): void {
    $colVis = { ...$colVis, [key]: checked };
  }
</script>

<div class="card bg-dark border-secondary mb-3 control-panel">
  <div class="card-body">
    <h2 class="h5">Visible Columns</h2>
    <div class="row g-3">
      {#each deviceColumns as column (column.key)}
        <div class="col-md-4">
          <label class="d-flex align-items-center gap-2">
            <input
              class="form-check-input"
              type="checkbox"
              checked={$colVis[column.key] ?? false}
              on:change={(e) =>
                toggleColumn(column.key, (e.currentTarget as HTMLInputElement).checked)}
            />
            <span>{column.label}</span>
          </label>
        </div>
      {/each}
    </div>
  </div>
</div>

<style>
  .control-panel :global(.card-body) {
    padding-top: 1rem;
  }
</style>
