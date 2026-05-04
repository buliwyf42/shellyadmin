<script lang="ts">
  import type { SysState } from './types';
  import SectionCard from '../../components/SectionCard.svelte';
  import FieldRow from '../../components/FieldRow.svelte';
  import Toggle from '../../components/Toggle.svelte';

  export let state: SysState;

  $: expanded =
    state.nameEnabled ||
    state.tzEnabled ||
    state.latEnabled ||
    state.lonEnabled ||
    state.sntpEnabled ||
    state.debugWSEnabled ||
    state.debugMQTTEnabled ||
    state.debugUDPHostEnabled ||
    state.rpcUDPPortEnabled ||
    state.ecoEnabled ||
    state.discoverableEnabled;
</script>

<SectionCard tag="sys" title="System & Location" bind:open={state.open} forceOpen={expanded}>
  <div class="sa-form-grid">
    <div data-span="6">
      <FieldRow label="Device Name" bind:enabled={state.nameEnabled}>
        <input class="form-control" bind:value={state.name} disabled={!state.nameEnabled} />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow label="Timezone" bind:enabled={state.tzEnabled}>
        <input class="form-control" bind:value={state.tz} disabled={!state.tzEnabled} />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow label="SNTP Server" bind:enabled={state.sntpEnabled}>
        <input class="form-control" bind:value={state.sntp} disabled={!state.sntpEnabled} />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow label="Debug WebSocket (stream logs)" bind:enabled={state.debugWSEnabled}>
        <Toggle
          bind:checked={state.debugWS}
          disabled={!state.debugWSEnabled}
          label={state.debugWS ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow label="Debug MQTT" bind:enabled={state.debugMQTTEnabled}>
        <Toggle
          bind:checked={state.debugMQTT}
          disabled={!state.debugMQTTEnabled}
          label={state.debugMQTT ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow label="Debug UDP Host" bind:enabled={state.debugUDPHostEnabled}>
        <input
          class="form-control"
          placeholder="host:port"
          bind:value={state.debugUDPHost}
          disabled={!state.debugUDPHostEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow label="RPC UDP Port (0 = off)" bind:enabled={state.rpcUDPPortEnabled}>
        <input
          class="form-control"
          type="number"
          min="0"
          bind:value={state.rpcUDPPort}
          disabled={!state.rpcUDPPortEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow label="Latitude" bind:enabled={state.latEnabled}>
        <input
          class="form-control"
          type="number"
          step="0.0001"
          bind:value={state.lat}
          disabled={!state.latEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow label="Longitude" bind:enabled={state.lonEnabled}>
        <input
          class="form-control"
          type="number"
          step="0.0001"
          bind:value={state.lon}
          disabled={!state.lonEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow label="Eco Mode" bind:enabled={state.ecoEnabled}>
        <Toggle
          bind:checked={state.eco}
          disabled={!state.ecoEnabled}
          label={state.eco ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow label="Discoverable" bind:enabled={state.discoverableEnabled}>
        <Toggle
          bind:checked={state.discoverable}
          disabled={!state.discoverableEnabled}
          label={state.discoverable ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
  </div>
</SectionCard>
