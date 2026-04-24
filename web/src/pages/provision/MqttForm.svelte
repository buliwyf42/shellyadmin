<script lang="ts">
  import type { MqttState } from './types';
  import { sslCAOptions } from './sslCa';
  import SectionCard from '../../components/SectionCard.svelte';
  import FieldRow from '../../components/FieldRow.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import Select from '../../components/Select.svelte';

  export let state: MqttState;

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
    state.useClientCertEnabled;
</script>

<SectionCard
  tag="mqtt"
  title="MQTT Broker"
  bind:open={state.open}
  forceOpen={expanded}
  bind:enabled={state.enabled}
>
  <div class="sa-form-grid">
    <div data-span="6">
      <FieldRow label="Enable MQTT" bind:enabled={state.enableField} disabled={!state.enabled}>
        <Toggle
          bind:checked={state.enable}
          disabled={!state.enabled || !state.enableField}
          label={state.enable ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow label="Broker" bind:enabled={state.serverEnabled} disabled={!state.enabled}>
        <input
          class="form-control"
          bind:value={state.server}
          disabled={!state.enabled || !state.serverEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow label="Client ID" bind:enabled={state.clientIDEnabled} disabled={!state.enabled}>
        <input
          class="form-control"
          bind:value={state.clientID}
          disabled={!state.enabled || !state.clientIDEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow
        label="Topic Prefix"
        bind:enabled={state.topicPrefixEnabled}
        disabled={!state.enabled}
      >
        <input
          class="form-control"
          bind:value={state.topicPrefix}
          disabled={!state.enabled || !state.topicPrefixEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="Username" bind:enabled={state.userEnabled} disabled={!state.enabled}>
        <input
          class="form-control"
          bind:value={state.user}
          disabled={!state.enabled || !state.userEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="Password" bind:enabled={state.passEnabled} disabled={!state.enabled}>
        <input
          class="form-control"
          type="password"
          bind:value={state.pass}
          disabled={!state.enabled || !state.passEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="SSL CA" bind:enabled={state.sslCAEnabled} disabled={!state.enabled}>
        <Select
          bind:value={state.sslCA}
          options={sslCAOptions}
          disabled={!state.enabled || !state.sslCAEnabled}
          ariaLabel="SSL CA"
        />
      </FieldRow>
    </div>
    <div data-span="3">
      <FieldRow
        label="RPC notifications"
        bind:enabled={state.rpcNtfEnabled}
        disabled={!state.enabled}
      >
        <Toggle
          bind:checked={state.rpcNtf}
          disabled={!state.enabled || !state.rpcNtfEnabled}
          label={state.rpcNtf ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
    <div data-span="3">
      <FieldRow
        label="Status updates"
        bind:enabled={state.statusNtfEnabled}
        disabled={!state.enabled}
      >
        <Toggle
          bind:checked={state.statusNtf}
          disabled={!state.enabled || !state.statusNtfEnabled}
          label={state.statusNtf ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
    <div data-span="3">
      <FieldRow label="Enable RPC" bind:enabled={state.enableRPCEnabled} disabled={!state.enabled}>
        <Toggle
          bind:checked={state.enableRPC}
          disabled={!state.enabled || !state.enableRPCEnabled}
          label={state.enableRPC ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
    <div data-span="3">
      <FieldRow
        label="Enable control"
        bind:enabled={state.enableControlEnabled}
        disabled={!state.enabled}
      >
        <Toggle
          bind:checked={state.enableControl}
          disabled={!state.enabled || !state.enableControlEnabled}
          label={state.enableControl ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow
        label="Use Client Certificate"
        bind:enabled={state.useClientCertEnabled}
        disabled={!state.enabled}
      >
        <Toggle
          bind:checked={state.useClientCert}
          disabled={!state.enabled || !state.useClientCertEnabled}
          label={state.useClientCert ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
  </div>
</SectionCard>
