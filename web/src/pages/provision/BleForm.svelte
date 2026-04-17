<script lang="ts">
  import type { BleState } from './types'

  export let state: BleState

  $: expanded = state.enabled || state.enableField || state.rpcEnabledField || state.observerEnabledField
  $: visible = expanded || state.open
</script>

<div class="card bg-black border-secondary">
  <div class="card-body">
    <div
      class="d-flex justify-content-between align-items-center mb-3"
      role="button"
      tabindex="0"
      on:click={() => (state.open = !state.open)}
      on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (state.open = !state.open)}
      style="cursor: pointer"
    >
      <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer">
        <input type="checkbox" class="form-check-input" bind:checked={state.enabled} on:click|stopPropagation />
        <strong>ble</strong> - Bluetooth (Gen 2+)
      </label>
      <span class="text-secondary">{visible ? '▾' : '▸'}</span>
    </div>
    {#if visible}
      <div class="row g-2">
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.enableField} disabled={!state.enabled} />
            Enable BLE
          </label>
          <select class="form-select" bind:value={state.enable} disabled={!state.enabled || !state.enableField}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.rpcEnabledField} disabled={!state.enabled} />
            Enable RPC over BLE
          </label>
          <select class="form-select" bind:value={state.rpcEnabled} disabled={!state.enabled || !state.rpcEnabledField}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.observerEnabledField} disabled={!state.enabled} />
            Observer Mode
          </label>
          <select class="form-select" bind:value={state.observerEnabled} disabled={!state.enabled || !state.observerEnabledField}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
      </div>
    {/if}
  </div>
</div>
