<script lang="ts">
  import type { ModbusState } from './types'
  import SectionCard from '../../components/SectionCard.svelte'
  import FieldRow from '../../components/FieldRow.svelte'
  import Toggle from '../../components/Toggle.svelte'

  export let state: ModbusState

  $: expanded = state.enabled || state.enableField
</script>

<SectionCard tag="modbus" title="Modbus-TCP (Pro only)" bind:open={state.open} forceOpen={expanded} bind:enabled={state.enabled}>
  <div class="sa-form-grid">
    <div data-span="4">
      <FieldRow label="Enable Modbus" bind:enabled={state.enableField} disabled={!state.enabled}>
        <Toggle bind:checked={state.enable} disabled={!state.enabled || !state.enableField} label={state.enable ? 'On' : 'Off'} />
      </FieldRow>
    </div>
  </div>
</SectionCard>
