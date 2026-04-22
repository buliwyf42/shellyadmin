<script lang="ts">
  import type { UIState } from './types';
  import SectionCard from '../../components/SectionCard.svelte';
  import FieldRow from '../../components/FieldRow.svelte';

  export let state: UIState;

  $: expanded = state.enabled || state.idleBrightnessEnabled;
</script>

<SectionCard
  tag="ui"
  title="UI (Display devices only)"
  bind:open={state.open}
  forceOpen={expanded}
  bind:enabled={state.enabled}
>
  <div class="sa-form-grid">
    <div data-span="4">
      <FieldRow
        label="Idle Brightness"
        bind:enabled={state.idleBrightnessEnabled}
        disabled={!state.enabled}
        help="0–100, display brightness when idle"
      >
        <input
          class="form-control"
          type="number"
          min="0"
          max="100"
          bind:value={state.idleBrightness}
          disabled={!state.enabled || !state.idleBrightnessEnabled}
        />
      </FieldRow>
    </div>
  </div>
</SectionCard>
