<script lang="ts">
  import type { ZigbeeState } from './types';
  import SectionCard from '../../components/SectionCard.svelte';
  import FieldRow from '../../components/FieldRow.svelte';
  import Toggle from '../../components/Toggle.svelte';

  export let state: ZigbeeState;

  $: expanded = state.enableField;
</script>

<SectionCard
  tag="zigbee"
  title="Zigbee (Gen3 mini only)"
  bind:open={state.open}
  forceOpen={expanded}
>
  <div class="sa-form-grid">
    <div data-span="4">
      <FieldRow label="Enable Zigbee" bind:enabled={state.enableField}>
        <Toggle
          bind:checked={state.enable}
          disabled={!state.enableField}
          label={state.enable ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
  </div>
</SectionCard>
