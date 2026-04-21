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

  const ipv6ModeOptions: Array<{ value: EthState['ipv6Mode']; label: string }> = [
    { value: 'disabled', label: 'Disabled' },
    { value: 'slaac', label: 'SLAAC' },
  ]

  $: expanded =
    state.enabled ||
    state.enableField ||
    state.ipv4ModeEnabled ||
    state.ipEnabled ||
    state.netmaskEnabled ||
    state.gwEnabled ||
    state.nameserverEnabled ||
    state.ipv6Enabled
  $: staticDisabled = !state.enabled || state.ipv4Mode !== 'static'
  $: ipv6FieldsDisabled = !state.enabled || !state.ipv6Enabled
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

  <div class="sa-eth-ipv6">
    <div class="sa-eth-ipv6-heading">
      <label class="sa-eth-ipv6-toggle">
        <input type="checkbox" bind:checked={state.ipv6Enabled} disabled={!state.enabled} />
        Configure IPv6
      </label>
    </div>
    {#if state.ipv6Enabled}
      <div class="sa-form-grid">
        <div data-span="4">
          <FieldRow label="IPv6 Mode" disabled={ipv6FieldsDisabled}>
            <Select bind:value={state.ipv6Mode} options={ipv6ModeOptions} disabled={ipv6FieldsDisabled} ariaLabel="IPv6 Mode" />
          </FieldRow>
        </div>
        <div data-span="4">
          <FieldRow label="IPv6 Address" bind:enabled={state.ipv6IpEnabled} disabled={ipv6FieldsDisabled}>
            <input class="form-control" bind:value={state.ipv6Ip} disabled={ipv6FieldsDisabled || !state.ipv6IpEnabled} />
          </FieldRow>
        </div>
        <div data-span="4">
          <FieldRow label="IPv6 Netmask" bind:enabled={state.ipv6NetmaskEnabled} disabled={ipv6FieldsDisabled}>
            <input class="form-control" bind:value={state.ipv6Netmask} disabled={ipv6FieldsDisabled || !state.ipv6NetmaskEnabled} />
          </FieldRow>
        </div>
        <div data-span="4">
          <FieldRow label="IPv6 Gateway" bind:enabled={state.ipv6GwEnabled} disabled={ipv6FieldsDisabled}>
            <input class="form-control" bind:value={state.ipv6Gw} disabled={ipv6FieldsDisabled || !state.ipv6GwEnabled} />
          </FieldRow>
        </div>
        <div data-span="4">
          <FieldRow label="IPv6 Nameserver" bind:enabled={state.ipv6NameserverEnabled} disabled={ipv6FieldsDisabled}>
            <input class="form-control" bind:value={state.ipv6Nameserver} disabled={ipv6FieldsDisabled || !state.ipv6NameserverEnabled} />
          </FieldRow>
        </div>
      </div>
    {/if}
  </div>
</SectionCard>

<style>
  .sa-eth-ipv6 {
    margin-top: var(--space-3);
    padding-top: var(--space-3);
    border-top: 1px solid var(--border-soft);
  }
  .sa-eth-ipv6-heading {
    margin-bottom: var(--space-3);
  }
  .sa-eth-ipv6-toggle {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: 0.82rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--muted);
    cursor: pointer;
  }
  .sa-eth-ipv6-toggle input[type='checkbox'] {
    cursor: pointer;
  }
</style>
