<script lang="ts">
  import type { SysState } from './types'

  export let state: SysState

  $: expanded =
    state.enabled ||
    state.nameEnabled ||
    state.tzEnabled ||
    state.latEnabled ||
    state.lonEnabled ||
    state.sntpEnabled ||
    state.debugWSEnabled ||
    state.debugUDPHostEnabled ||
    state.rpcUDPPortEnabled ||
    state.ecoEnabled ||
    state.discoverableEnabled
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
        <strong>sys</strong> - System & Location
      </label>
      <span class="text-secondary">{visible ? '▾' : '▸'}</span>
    </div>
    {#if visible}
      <div class="row g-2">
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.nameEnabled} disabled={!state.enabled} />
            Device Name
          </label>
          <input class="form-control" bind:value={state.name} disabled={!state.enabled || !state.nameEnabled} />
        </div>
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.tzEnabled} disabled={!state.enabled} />
            Timezone
          </label>
          <input class="form-control" bind:value={state.tz} disabled={!state.enabled || !state.tzEnabled} />
        </div>
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.sntpEnabled} disabled={!state.enabled} />
            SNTP Server
          </label>
          <input class="form-control" bind:value={state.sntp} disabled={!state.enabled || !state.sntpEnabled} />
        </div>
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.debugWSEnabled} disabled={!state.enabled} />
            Debug WebSocket (stream logs)
          </label>
          <select class="form-select" bind:value={state.debugWS} disabled={!state.enabled || !state.debugWSEnabled}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.debugUDPHostEnabled} disabled={!state.enabled} />
            Debug UDP Host
          </label>
          <input class="form-control" placeholder="host:port" bind:value={state.debugUDPHost} disabled={!state.enabled || !state.debugUDPHostEnabled} />
        </div>
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.rpcUDPPortEnabled} disabled={!state.enabled} />
            RPC UDP Port (0=off)
          </label>
          <input class="form-control" type="number" min="0" bind:value={state.rpcUDPPort} disabled={!state.enabled || !state.rpcUDPPortEnabled} />
        </div>
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.latEnabled} disabled={!state.enabled} />
            Latitude
          </label>
          <input class="form-control" type="number" step="0.0001" bind:value={state.lat} disabled={!state.enabled || !state.latEnabled} />
        </div>
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.lonEnabled} disabled={!state.enabled} />
            Longitude
          </label>
          <input class="form-control" type="number" step="0.0001" bind:value={state.lon} disabled={!state.enabled || !state.lonEnabled} />
        </div>
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.ecoEnabled} disabled={!state.enabled} />
            Eco Mode
          </label>
          <select class="form-select" bind:value={state.eco} disabled={!state.enabled || !state.ecoEnabled}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.discoverableEnabled} disabled={!state.enabled} />
            Discoverable
          </label>
          <select class="form-select" bind:value={state.discoverable} disabled={!state.enabled || !state.discoverableEnabled}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
      </div>
    {/if}
  </div>
</div>
