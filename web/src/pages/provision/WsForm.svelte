<script lang="ts">
  import type { WsState } from './types';
  import { isTLSServerURL } from './state';
  import { sslCAOptions } from './sslCa';
  import SectionCard from '../../components/SectionCard.svelte';
  import FieldRow from '../../components/FieldRow.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import Select from '../../components/Select.svelte';

  export let state: WsState;

  const tlsModeOptions: Array<{ value: WsState['tlsMode']; label: string }> = [
    { value: 'no_validation', label: 'TLS — no validation' },
    { value: 'default', label: 'TLS — default' },
    { value: 'user', label: 'TLS — user CA' },
  ];

  $: expanded =
    state.enableField || state.serverEnabled || state.tlsModeEnabled || state.sslCAEnabled;
</script>

<SectionCard tag="ws" title="WebSocket (Gen 2+)" bind:open={state.open} forceOpen={expanded}>
  <div class="sa-form-grid">
    <div data-span="4">
      <FieldRow label="Enable WebSocket" bind:enabled={state.enableField}>
        <Toggle
          bind:checked={state.enable}
          disabled={!state.enableField}
          label={state.enable ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="Server URL" bind:enabled={state.serverEnabled}>
        <input class="form-control" bind:value={state.server} disabled={!state.serverEnabled} />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="Connection type" bind:enabled={state.tlsModeEnabled}>
        <Select
          bind:value={state.tlsMode}
          options={tlsModeOptions}
          disabled={!state.tlsModeEnabled}
          ariaLabel="Connection type"
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="SSL CA" bind:enabled={state.sslCAEnabled}>
        <Select
          bind:value={state.sslCA}
          options={sslCAOptions}
          disabled={!state.sslCAEnabled || state.tlsMode !== 'user'}
          ariaLabel="SSL CA"
        />
      </FieldRow>
    </div>
  </div>
  {#if state.serverEnabled && state.server && !isTLSServerURL(state.server)}
    <div class="text-secondary mt-2 text-hint-sm">
      TLS settings are ignored for plain <code>ws://</code> endpoints. Use <code>wss://</code> for TLS.
    </div>
  {/if}
</SectionCard>
