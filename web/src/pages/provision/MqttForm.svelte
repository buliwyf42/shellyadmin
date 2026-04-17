<script lang="ts">
  import type { MqttState } from './types'

  export let state: MqttState

  $: expanded =
    state.enabled ||
    state.enableField ||
    state.serverEnabled ||
    state.clientIDEnabled ||
    state.topicPrefixEnabled ||
    state.userEnabled ||
    state.passEnabled ||
    state.sslCAEnabled ||
    state.rpcNtfEnabled ||
    state.statusNtfEnabled ||
    state.enableRPCEnabled ||
    state.enableControlEnabled ||
    state.useClientCertEnabled
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
        <strong>mqtt</strong> - MQTT Broker
      </label>
      <span class="text-secondary">{visible ? '▾' : '▸'}</span>
    </div>
    {#if visible}
      <div class="row g-2">
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.enableField} disabled={!state.enabled} />
            Enable MQTT
          </label>
          <select class="form-select" bind:value={state.enable} disabled={!state.enabled || !state.enableField}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.serverEnabled} disabled={!state.enabled} />
            Broker
          </label>
          <input class="form-control" bind:value={state.server} disabled={!state.enabled || !state.serverEnabled} />
        </div>
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.clientIDEnabled} disabled={!state.enabled} />
            Client ID
          </label>
          <input class="form-control" bind:value={state.clientID} disabled={!state.enabled || !state.clientIDEnabled} />
        </div>
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.topicPrefixEnabled} disabled={!state.enabled} />
            Topic Prefix
          </label>
          <input class="form-control" bind:value={state.topicPrefix} disabled={!state.enabled || !state.topicPrefixEnabled} />
        </div>
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.userEnabled} disabled={!state.enabled} />
            Username
          </label>
          <input class="form-control" bind:value={state.user} disabled={!state.enabled || !state.userEnabled} />
        </div>
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.passEnabled} disabled={!state.enabled} />
            Password
          </label>
          <input class="form-control" type="password" bind:value={state.pass} disabled={!state.enabled || !state.passEnabled} />
        </div>
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.sslCAEnabled} disabled={!state.enabled} />
            SSL CA
          </label>
          <select class="form-select" bind:value={state.sslCA} disabled={!state.enabled || !state.sslCAEnabled}>
            <option value="">— none (no TLS) —</option>
            <option value="*">* (disable cert validation)</option>
            <option value="ca.pem">ca.pem (built-in CA)</option>
            <option value="user_ca.pem">user_ca.pem (user CA)</option>
          </select>
        </div>
        <div class="col-md-3">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.rpcNtfEnabled} disabled={!state.enabled} />
            RPC notifications
          </label>
          <select class="form-select" bind:value={state.rpcNtf} disabled={!state.enabled || !state.rpcNtfEnabled}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
        <div class="col-md-3">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.statusNtfEnabled} disabled={!state.enabled} />
            Status updates
          </label>
          <select class="form-select" bind:value={state.statusNtf} disabled={!state.enabled || !state.statusNtfEnabled}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
        <div class="col-md-3">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.enableRPCEnabled} disabled={!state.enabled} />
            Enable RPC
          </label>
          <select class="form-select" bind:value={state.enableRPC} disabled={!state.enabled || !state.enableRPCEnabled}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
        <div class="col-md-3">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.enableControlEnabled} disabled={!state.enabled} />
            Enable control
          </label>
          <select class="form-select" bind:value={state.enableControl} disabled={!state.enabled || !state.enableControlEnabled}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={state.useClientCertEnabled} disabled={!state.enabled} />
            Use Client Certificate
          </label>
          <select class="form-select" bind:value={state.useClientCert} disabled={!state.enabled || !state.useClientCertEnabled}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
      </div>
    {/if}
  </div>
</div>
