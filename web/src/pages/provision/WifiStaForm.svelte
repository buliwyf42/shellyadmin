<script lang="ts">
  import type { WifiStaEntry } from './types';
  import FieldRow from '../../components/FieldRow.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import Select from '../../components/Select.svelte';

  export let label: string;
  export let value: WifiStaEntry;
  export let disabled: boolean = false;

  const ipv4ModeOptions: Array<{ value: WifiStaEntry['ipv4mode']; label: string }> = [
    { value: 'dhcp', label: 'DHCP' },
    { value: 'static', label: 'Static' },
  ];

  $: staticDisabled = disabled || !value.ipv4ModeEnabled || value.ipv4mode !== 'static';
</script>

<div class="sa-wifi-sta">
  <div class="sa-wifi-sta-heading">{label}</div>
  <div class="sa-form-grid">
    <div data-span="4">
      <FieldRow label="Enable" bind:enabled={value.enableField} {disabled}>
        <Toggle
          bind:checked={value.enable}
          disabled={disabled || !value.enableField}
          label={value.enable ? 'On' : 'Off'}
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="SSID" bind:enabled={value.ssidEnabled} {disabled}>
        <input
          class="form-control"
          bind:value={value.ssid}
          disabled={disabled || !value.ssidEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="Password" bind:enabled={value.passEnabled} {disabled}>
        <input
          class="form-control"
          type="password"
          bind:value={value.pass}
          disabled={disabled || !value.passEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="IPv4 Mode" bind:enabled={value.ipv4ModeEnabled} {disabled}>
        <Select
          bind:value={value.ipv4mode}
          options={ipv4ModeOptions}
          disabled={disabled || !value.ipv4ModeEnabled}
          ariaLabel="IPv4 Mode"
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="IP" bind:enabled={value.ipEnabled} disabled={staticDisabled}>
        <input
          class="form-control"
          bind:value={value.ip}
          disabled={staticDisabled || !value.ipEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="Netmask" bind:enabled={value.netmaskEnabled} disabled={staticDisabled}>
        <input
          class="form-control"
          bind:value={value.netmask}
          disabled={staticDisabled || !value.netmaskEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="Gateway" bind:enabled={value.gwEnabled} disabled={staticDisabled}>
        <input
          class="form-control"
          bind:value={value.gw}
          disabled={staticDisabled || !value.gwEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="Nameserver" bind:enabled={value.nameserverEnabled} disabled={staticDisabled}>
        <input
          class="form-control"
          bind:value={value.nameserver}
          disabled={staticDisabled || !value.nameserverEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="Hostname" bind:enabled={value.hostnameEnabled} {disabled}>
        <input
          class="form-control"
          bind:value={value.hostname}
          placeholder="{'{device_name}'}"
          disabled={disabled || !value.hostnameEnabled}
        />
      </FieldRow>
    </div>
  </div>
</div>

<style>
  .sa-wifi-sta {
    margin-top: var(--space-3);
    padding-top: var(--space-3);
    border-top: 1px solid var(--border-soft);
  }
  .sa-wifi-sta-heading {
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--muted);
    margin-bottom: var(--space-3);
  }
</style>
