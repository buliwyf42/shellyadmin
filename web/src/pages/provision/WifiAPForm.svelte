<script lang="ts">
  import type { WifiAPState } from './types'
  import SectionCard from '../../components/SectionCard.svelte'
  import FieldRow from '../../components/FieldRow.svelte'
  import Toggle from '../../components/Toggle.svelte'

  export let state: WifiAPState

  $: expanded = state.enabled || state.enableField || state.ssidEnabled || state.passEnabled || state.isOpenField
</script>

<SectionCard tag="wifi ap" title="WiFi AP Hotspot" bind:open={state.open} forceOpen={expanded} bind:enabled={state.enabled}>
  <div class="sa-form-grid">
    <div data-span="4">
      <FieldRow label="Enable AP" bind:enabled={state.enableField} disabled={!state.enabled}>
        <Toggle bind:checked={state.enable} disabled={!state.enabled || !state.enableField} label={state.enable ? 'On' : 'Off'} />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="Open AP (no password)" bind:enabled={state.isOpenField} disabled={!state.enabled}>
        <Toggle bind:checked={state.isOpen} disabled={!state.enabled || !state.isOpenField} label={state.isOpen ? 'On' : 'Off'} />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="SSID" bind:enabled={state.ssidEnabled} disabled={!state.enabled}>
        <input class="form-control" bind:value={state.ssid} disabled={!state.enabled || !state.ssidEnabled} />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="Password" bind:enabled={state.passEnabled} disabled={!state.enabled}>
        <input class="form-control" type="password" bind:value={state.pass} disabled={!state.enabled || !state.passEnabled} />
      </FieldRow>
    </div>
  </div>
</SectionCard>
