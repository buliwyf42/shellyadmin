<script lang="ts">
  import type { AuthState, CloudState, MatterState, UIState, WifiState } from './types';
  import SectionCard from '../../components/SectionCard.svelte';
  import FieldRow from '../../components/FieldRow.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import WifiStaForm from './WifiStaForm.svelte';
  import WifiRoamForm from './WifiRoamForm.svelte';
  import UIConfigForm from './UIConfigForm.svelte';

  export let matter: MatterState;
  export let cloud: CloudState;
  export let auth: AuthState;
  export let wifi: WifiState;
  export let ui: UIState;

  $: matterExpanded = matter.enabled || matter.enableField;
  $: cloudExpanded = cloud.enabled || cloud.enableField;
  $: authExpanded = auth.enabled || auth.passEnabled;
  $: wifiExpanded =
    wifi.enabled ||
    wifi.staEnabled ||
    wifi.sta.ssidEnabled ||
    wifi.sta.passEnabled ||
    wifi.sta1Enabled ||
    wifi.roamEnabled;
  $: uiExpanded = ui.enabled || ui.idleBrightnessEnabled;
</script>

<SectionCard
  tag="matter"
  title="Matter (Gen 2+)"
  bind:open={matter.open}
  forceOpen={matterExpanded}
  bind:enabled={matter.enabled}
>
  <div class="sa-form-grid">
    <div data-span="4">
      <FieldRow label="Enable Matter" bind:enabled={matter.enableField} disabled={!matter.enabled}>
        <Toggle
          bind:checked={matter.enable}
          disabled={!matter.enabled || !matter.enableField}
          label={matter.enable ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
  </div>
</SectionCard>

<SectionCard
  tag="cloud"
  title="Shelly Cloud"
  bind:open={cloud.open}
  forceOpen={cloudExpanded}
  bind:enabled={cloud.enabled}
>
  <div class="sa-form-grid">
    <div data-span="4">
      <FieldRow label="Enable Cloud" bind:enabled={cloud.enableField} disabled={!cloud.enabled}>
        <Toggle
          bind:checked={cloud.enable}
          disabled={!cloud.enabled || !cloud.enableField}
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
  bind:enabled={auth.enabled}
>
  <div class="sa-form-grid">
    <div data-span="6">
      <FieldRow label="Password" bind:enabled={auth.passEnabled} disabled={!auth.enabled}>
        <input
          class="form-control"
          type="password"
          bind:value={auth.pass}
          disabled={!auth.enabled || !auth.passEnabled}
        />
      </FieldRow>
    </div>
  </div>
</SectionCard>

<UIConfigForm bind:state={ui} />

<SectionCard
  tag="wifi"
  title="WiFi STA"
  bind:open={wifi.open}
  forceOpen={wifiExpanded}
  bind:enabled={wifi.enabled}
>
  <div class="sa-form-grid">
    <div data-span="4">
      <FieldRow
        label="Configure primary STA"
        bind:enabled={wifi.staEnabled}
        disabled={!wifi.enabled}
      />
    </div>
    <div data-span="4">
      <FieldRow
        label="Configure secondary STA (STA1)"
        bind:enabled={wifi.sta1Enabled}
        disabled={!wifi.enabled}
      />
    </div>
    <div data-span="4">
      <FieldRow
        label="Configure roaming"
        bind:enabled={wifi.roamEnabled}
        disabled={!wifi.enabled}
      />
    </div>
  </div>
  {#if wifi.staEnabled}
    <WifiStaForm label="Primary STA" bind:value={wifi.sta} disabled={!wifi.enabled} />
  {/if}
  {#if wifi.sta1Enabled}
    <WifiStaForm label="Secondary STA (STA1)" bind:value={wifi.sta1} disabled={!wifi.enabled} />
  {/if}
  {#if wifi.roamEnabled}
    <WifiRoamForm bind:value={wifi.roam} disabled={!wifi.enabled} />
  {/if}
</SectionCard>
