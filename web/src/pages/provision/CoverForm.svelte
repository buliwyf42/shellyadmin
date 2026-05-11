<script lang="ts">
  import type { CoverState } from './types';
  import SectionCard from '../../components/SectionCard.svelte';
  import FieldRow from '../../components/FieldRow.svelte';
  import Toggle from '../../components/Toggle.svelte';

  export let state: CoverState;

  $: expanded =
    state.nameEnabled ||
    state.maxtimeOpenEnabled ||
    state.maxtimeCloseEnabled ||
    state.swapInputsField ||
    state.powerLimitEnabled ||
    state.slatEnabled;
</script>

<SectionCard
  tag="cover"
  title="Cover (blinds / shutters)"
  bind:open={state.open}
  forceOpen={expanded}
>
  <div class="sa-cover-notice">
    Configures one cover component on devices that expose <code>Cover.SetConfig</code> (Shelly Pro
    Shutter / Pro 2PM in shutter mode, Plus 2PM in cover profile). For multi-cover devices, target a
    specific instance with <code>id</code>. The <code>slat</code> sub-object is the FW 2.0.0-beta1 addition
    for venetian-blind tilt. Advanced fields (obstruction_detection, motor, safety_switch) stay JSON-editor
    only.
  </div>

  <div class="sa-form-grid">
    <div data-span="3">
      <label class="form-label" for="sa-cover-id">Component ID</label>
      <input id="sa-cover-id" class="form-control" type="number" min="0" bind:value={state.id} />
    </div>
    <div data-span="9">
      <FieldRow label="Name" bind:enabled={state.nameEnabled}>
        <input
          class="form-control"
          bind:value={state.name}
          disabled={!state.nameEnabled}
          placeholder="e.g. Living-Room Blind"
        />
      </FieldRow>
    </div>

    <div data-span="6">
      <FieldRow label="Max travel time (open, seconds)" bind:enabled={state.maxtimeOpenEnabled}>
        <input
          class="form-control"
          type="number"
          step="0.1"
          min="0"
          bind:value={state.maxtimeOpen}
          disabled={!state.maxtimeOpenEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow label="Max travel time (close, seconds)" bind:enabled={state.maxtimeCloseEnabled}>
        <input
          class="form-control"
          type="number"
          step="0.1"
          min="0"
          bind:value={state.maxtimeClose}
          disabled={!state.maxtimeCloseEnabled}
        />
      </FieldRow>
    </div>

    <div data-span="6">
      <FieldRow
        label="Swap inputs (reverse open/close switches)"
        bind:enabled={state.swapInputsField}
      >
        <Toggle
          bind:checked={state.swapInputs}
          disabled={!state.swapInputsField}
          label={state.swapInputs ? 'Swapped' : 'Normal'}
        />
      </FieldRow>
    </div>
    <div data-span="6">
      <FieldRow label="Power limit (watts)" bind:enabled={state.powerLimitEnabled}>
        <input
          class="form-control"
          type="number"
          step="1"
          min="0"
          bind:value={state.powerLimit}
          disabled={!state.powerLimitEnabled}
        />
      </FieldRow>
    </div>
  </div>

  <div class="sa-cover-slat">
    <FieldRow label="Slat / Tilt configuration (venetian blinds)" bind:enabled={state.slatEnabled}>
      <span class="text-secondary" style="font-size: 0.8rem;">
        {state.slatEnabled ? 'on — fields below ship' : 'off — slat block omitted from template'}
      </span>
    </FieldRow>

    {#if state.slatEnabled}
      <div class="sa-form-grid sa-cover-slat-grid">
        <div data-span="6">
          <FieldRow label="Slat enable" bind:enabled={state.slat.enableField}>
            <Toggle
              bind:checked={state.slat.enable}
              disabled={!state.slat.enableField}
              label={state.slat.enable ? 'On' : 'Off'}
            />
          </FieldRow>
        </div>
        <div data-span="6">
          <FieldRow label="Precise tilt control" bind:enabled={state.slat.preciseCtlField}>
            <Toggle
              bind:checked={state.slat.preciseCtl}
              disabled={!state.slat.preciseCtlField}
              label={state.slat.preciseCtl ? 'On' : 'Off'}
            />
          </FieldRow>
        </div>
        <div data-span="6">
          <FieldRow label="Slat open time (seconds)" bind:enabled={state.slat.openTimeEnabled}>
            <input
              class="form-control"
              type="number"
              step="0.1"
              min="0"
              bind:value={state.slat.openTime}
              disabled={!state.slat.openTimeEnabled}
            />
          </FieldRow>
        </div>
        <div data-span="6">
          <FieldRow label="Slat close time (seconds)" bind:enabled={state.slat.closeTimeEnabled}>
            <input
              class="form-control"
              type="number"
              step="0.1"
              min="0"
              bind:value={state.slat.closeTime}
              disabled={!state.slat.closeTimeEnabled}
            />
          </FieldRow>
        </div>
        <div data-span="6">
          <FieldRow label="Retain last tilt position" bind:enabled={state.slat.retainPosField}>
            <Toggle
              bind:checked={state.slat.retainPos}
              disabled={!state.slat.retainPosField}
              label={state.slat.retainPos ? 'On' : 'Off'}
            />
          </FieldRow>
        </div>
        <div data-span="6">
          <FieldRow label="Step size (percentage points)" bind:enabled={state.slat.stepPosEnabled}>
            <input
              class="form-control"
              type="number"
              step="1"
              min="1"
              max="100"
              bind:value={state.slat.stepPos}
              disabled={!state.slat.stepPosEnabled}
            />
          </FieldRow>
        </div>
      </div>
    {/if}
  </div>
</SectionCard>

<style>
  .sa-cover-notice {
    font-size: 0.8rem;
    color: var(--muted);
    margin-bottom: var(--space-3);
  }
  .sa-cover-slat {
    border-top: 1px solid var(--border);
    margin-top: var(--space-3);
    padding-top: var(--space-3);
  }
  .sa-cover-slat-grid {
    margin-top: var(--space-2);
  }
</style>
