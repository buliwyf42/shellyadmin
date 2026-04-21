<script lang="ts">
  import type { EthState } from './types'
  import SectionCard from '../../components/SectionCard.svelte'
  import FieldRow from '../../components/FieldRow.svelte'
  import Toggle from '../../components/Toggle.svelte'
  import Select from '../../components/Select.svelte'

  export let state: EthState

  const ipv4ModeOptions: Array<{ value: EthState['ipv4Mode']; label: string }> = [
    { value: 'dhcp', label: 'DHCP' },
    { value: 'static', label: 'Static' },
  ]

  $: expanded =
    state.enabled ||
    state.enableField ||
    state.ipv4ModeEnabled ||
    state.ipEnabled ||
    state.netmaskEnabled ||
    state.gwEnabled ||
    state.nameserverEnabled
  $: staticDisabled = !state.enabled || state.ipv4Mode !== 'static'
</script>

<SectionCard tag="eth" title="Ethernet (Pro/Plus only)" bind:open={state.open} forceOpen={expanded} bind:enabled={state.enabled}>
  <div class="sa-form-grid">
    <div data-span="4">
      <FieldRow label="Enable Eth" bind:enabled={state.enableField} disabled={!state.enabled}>
        <Toggle bind:checked={state.enable} disabled={!state.enabled || !state.enableField} label={state.enable ? 'On' : 'Off'} />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="IPv4 Mode" bind:enabled={state.ipv4ModeEnabled} disabled={!state.enabled}>
        <Select bind:value={state.ipv4Mode} options={ipv4ModeOptions} disabled={!state.enabled || !state.ipv4ModeEnabled} ariaLabel="IPv4 Mode" />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="IP" bind:enabled={state.ipEnabled} disabled={staticDisabled}>
        <input class="form-control" bind:value={state.ip} disabled={staticDisabled || !state.ipEnabled} />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="Netmask" bind:enabled={state.netmaskEnabled} disabled={staticDisabled}>
        <input class="form-control" bind:value={state.netmask} disabled={staticDisabled || !state.netmaskEnabled} />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="Gateway" bind:enabled={state.gwEnabled} disabled={staticDisabled}>
        <input class="form-control" bind:value={state.gw} disabled={staticDisabled || !state.gwEnabled} />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow label="Nameserver" bind:enabled={state.nameserverEnabled} disabled={staticDisabled}>
        <input class="form-control" bind:value={state.nameserver} disabled={staticDisabled || !state.nameserverEnabled} />
      </FieldRow>
    </div>
  </div>
</SectionCard>
