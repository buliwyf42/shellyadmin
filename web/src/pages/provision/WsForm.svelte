<script lang="ts">
  import type { WsState } from './types'
  import { isTLSServerURL } from './state'

  export let state: WsState

  $: expanded = state.enabled || state.enableField || state.serverEnabled || state.tlsModeEnabled || state.sslCAEnabled
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
        <strong>ws</strong> - WebSocket (Gen 2+)
      </label>
      <span class="text-secondary">{visible ? '▾' : '▸'}</span>
    </div>
    {#if visible}
      <div class="row g-2">
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.enableField} disabled={!state.enabled} />
            Enable WebSocket
          </label>
          <select class="form-select" bind:value={state.enable} disabled={!state.enabled || !state.enableField}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.serverEnabled} disabled={!state.enabled} />
            Server URL
          </label>
          <input class="form-control" bind:value={state.server} disabled={!state.enabled || !state.serverEnabled} />
        </div>
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.tlsModeEnabled} disabled={!state.enabled} />
            Connection type
          </label>
          <select class="form-select" bind:value={state.tlsMode} disabled={!state.enabled || !state.tlsModeEnabled}>
            <option value="no_validation">TLS no validation</option>
            <option value="default">Default TLS</option>
            <option value="user">User TLS</option>
          </select>
        </div>
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.sslCAEnabled} disabled={!state.enabled} />
            SSL CA
          </label>
          <input class="form-control" placeholder="* or ca.pem" bind:value={state.sslCA} disabled={!state.enabled || !state.sslCAEnabled || state.tlsMode !== 'user'} />
        </div>
      </div>
      {#if state.serverEnabled && state.server && !isTLSServerURL(state.server)}
        <div class="form-text mt-2">TLS settings are ignored for plain <code>ws://</code> endpoints. Use <code>wss://</code> for TLS.</div>
      {/if}
    {/if}
  </div>
</div>
