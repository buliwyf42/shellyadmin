<script lang="ts">
  import type { BleState } from './types';
  import SectionCard from '../../components/SectionCard.svelte';
  import FieldRow from '../../components/FieldRow.svelte';
  import Toggle from '../../components/Toggle.svelte';

  export let state: BleState;

  $: expanded = state.enabled || state.rpcEnabledField || state.observerEnabledField;
</script>

<SectionCard
  tag="ble"
  title="Bluetooth (Gen 2+)"
  bind:open={state.open}
  forceOpen={expanded}
  bind:enabled={state.enabled}
>
  <p class="sa-form-hint">
    Shelly firmware 2.0.0-beta1 removed the global BLE enable flag — Bluetooth now
    auto-activates whenever scanning is enabled. Use the toggles below to control
    the RPC and observer sub-systems.
  </p>
  <div class="sa-form-grid">
    <div data-span="6">
      <FieldRow
        label="Enable RPC over BLE"
        bind:enabled={state.rpcEnabledField}
        disabled={!state.enabled}
      >
        <Toggle
          bind:checked={state.rpcEnabled}
          disabled={!state.enabled || !state.rpcEnabledField}
          label={state.rpcEnabled ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow
        label="Observer Mode"
        bind:enabled={state.observerEnabledField}
        disabled={!state.enabled}
      >
        <Toggle
          bind:checked={state.observerEnabled}
          disabled={!state.enabled || !state.observerEnabledField}
          label={state.observerEnabled ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
  </div>
</SectionCard>
