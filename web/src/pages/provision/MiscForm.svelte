<script lang="ts">
  import type {
    AuthState,
    AutoUpdateState,
    CloudState,
    MatterState,
    UIState,
    WifiState,
  } from './types';
  import SectionCard from '../../components/SectionCard.svelte';
  import FieldRow from '../../components/FieldRow.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import WifiStaForm from './WifiStaForm.svelte';
  import WifiRoamForm from './WifiRoamForm.svelte';
  import UIConfigForm from './UIConfigForm.svelte';

  export let matter: MatterState;
  export let autoUpdate: AutoUpdateState;
  export let cloud: CloudState;
  export let auth: AuthState;
  export let wifi: WifiState;
  export let ui: UIState;

  $: matterExpanded = matter.enableField;
  $: autoUpdateExpanded = autoUpdate.enabled;
  $: cloudExpanded = cloud.enableField;
  $: authExpanded = auth.passEnabled;
  $: wifiExpanded =
    wifi.staEnabled ||
    wifi.sta.ssidEnabled ||
    wifi.sta.passEnabled ||
    wifi.sta1Enabled ||
    wifi.roamEnabled;
  $: uiExpanded = ui.idleBrightnessEnabled;
</script>

<SectionCard
  tag="auto_update"
  title="Auto-Update Schedule (Gen 2+)"
  bind:open={autoUpdate.open}
  forceOpen={autoUpdateExpanded}
>
  <div class="sa-form-grid">
    <div data-span="6">
      <FieldRow label="Configure auto-update" bind:enabled={autoUpdate.enabled}>
        <select class="form-select" bind:value={autoUpdate.mode} disabled={!autoUpdate.enabled}>
          <option value="off">Off (delete any existing schedule)</option>
          <option value="stable">Stable (nightly)</option>
          <option value="beta">Beta (nightly)</option>
        </select>
      </FieldRow>
    </div>
  </div>
  <p class="text-secondary small mb-0 mt-2">
    Synthesises a Schedule.* job that calls Shelly.Update nightly with origin="shelly_service" — the
    same mechanism the device's own web UI uses.
  </p>
</SectionCard>

<SectionCard
  tag="matter"
  title="Matter (Gen 2+)"
  bind:open={matter.open}
  forceOpen={matterExpanded}
>
  <div class="sa-form-grid">
    <div data-span="4">
      <FieldRow label="Enable Matter" bind:enabled={matter.enableField}>
        <Toggle
          bind:checked={matter.enable}
          disabled={!matter.enableField}
          label={matter.enable ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
  </div>
</SectionCard>

<SectionCard tag="cloud" title="Shelly Cloud" bind:open={cloud.open} forceOpen={cloudExpanded}>
  <div class="sa-form-grid">
    <div data-span="4">
      <FieldRow label="Enable Cloud" bind:enabled={cloud.enableField}>
        <Toggle
          bind:checked={cloud.enable}
          disabled={!cloud.enableField}
          label={cloud.enable ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
  </div>
</SectionCard>

<SectionCard
  tag="auth"
  title="Set Device Password (Gen 2+)"
  bind:open={auth.open}
  forceOpen={authExpanded}
>
  <div class="sa-form-grid">
    <div data-span="6">
      <FieldRow label="Password" bind:enabled={auth.passEnabled}>
        <input
          class="form-control"
          type="password"
          bind:value={auth.pass}
          disabled={!auth.passEnabled}
        />
      </FieldRow>
    </div>
  </div>
</SectionCard>

<UIConfigForm bind:state={ui} />

<SectionCard tag="wifi" title="WiFi STA" bind:open={wifi.open} forceOpen={wifiExpanded}>
  <div class="sa-form-grid">
    <div data-span="4">
      <FieldRow label="Configure primary STA" bind:enabled={wifi.staEnabled} />
    </div>
    <div data-span="4">
      <FieldRow label="Configure secondary STA (STA1)" bind:enabled={wifi.sta1Enabled} />
    </div>
    <div data-span="4">
      <FieldRow label="Configure roaming" bind:enabled={wifi.roamEnabled} />
    </div>
  </div>
  {#if wifi.staEnabled}
    <WifiStaForm label="Primary STA" bind:value={wifi.sta} />
  {/if}
  {#if wifi.sta1Enabled}
    <WifiStaForm label="Secondary STA (STA1)" bind:value={wifi.sta1} />
  {/if}
  {#if wifi.roamEnabled}
    <WifiRoamForm bind:value={wifi.roam} />
  {/if}
</SectionCard>
