<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import { devices } from '../lib/stores'
  import type { Device } from '../lib/types'
  import ComplianceBadge from '../components/ComplianceBadge.svelte'

  let filter = ''
  let selected = new Set<string>()
  let loading = false
  let error = ''
  let bulkAction = 'set_24h'
  let bulkValue = ''
  let bulkEnabled = true

  async function load(refresh = false) {
    loading = true
    error = ''
    try {
      $devices = refresh ? await api.refreshDevices() : await api.getDevices()
    } catch (err) {
      error = (err as Error).message
    } finally {
      loading = false
    }
  }

  async function applyBulk() {
    const macs = [...selected]
    const payload: Record<string, unknown> = { action: bulkAction, macs }
    if (bulkAction === 'set_location') Object.assign(payload, { lat: 52.52, lon: 13.4 })
    if (bulkAction === 'set_timezone' || bulkAction === 'set_mqtt_server') payload.value = bulkValue
    if (bulkAction === 'set_mqtt_enabled') payload.enabled = bulkEnabled
    await api.bulk(payload)
    await load(true)
  }

  function toggle(mac: string, checked: boolean) {
    checked ? selected.add(mac) : selected.delete(mac)
    selected = new Set(selected)
  }

  $: filtered = $devices.filter((d: Device) => {
    const haystack = `${d.name} ${d.ip} ${d.mac}`.toLowerCase()
    return haystack.includes(filter.toLowerCase())
  })

  onMount(() => void load())
</script>

<div class="d-flex justify-content-between align-items-center mb-3 gap-3 flex-wrap">
  <div class="d-flex gap-2 flex-wrap">
    <button class="btn btn-warning text-dark" on:click={() => load(true)} disabled={loading}>Refresh</button>
    <input class="form-control" placeholder="Filter name / IP / MAC" bind:value={filter} style="width: 18rem" />
  </div>
  <div class="d-flex gap-2 flex-wrap">
    <select class="form-select" bind:value={bulkAction}>
      <option value="set_24h">Set 24h</option>
      <option value="set_timezone">Set Timezone</option>
      <option value="set_mqtt_server">Set MQTT Broker</option>
      <option value="set_mqtt_enabled">Enable/Disable MQTT</option>
      <option value="set_location">Set Location</option>
    </select>
    {#if bulkAction === 'set_timezone' || bulkAction === 'set_mqtt_server'}
      <input class="form-control" bind:value={bulkValue} />
    {/if}
    {#if bulkAction === 'set_mqtt_enabled'}
      <select class="form-select" bind:value={bulkEnabled}>
        <option value={true}>Enable</option>
        <option value={false}>Disable</option>
      </select>
    {/if}
    <button class="btn btn-outline-light" on:click={applyBulk} disabled={selected.size === 0}>Apply to {selected.size}</button>
  </div>
</div>

{#if error}
  <div class="alert alert-danger">{error}</div>
{/if}

<div class="table-responsive">
  <table class="table table-dark table-striped align-middle">
    <thead>
      <tr>
        <th></th>
        <th>#</th>
        <th>Name</th>
        <th>IP</th>
        <th>Gen</th>
        <th>Model</th>
        <th>FW</th>
        <th>Online</th>
        <th>MQTT</th>
        <th>Cloud</th>
        <th>Timezone</th>
        <th>Compliance</th>
      </tr>
    </thead>
    <tbody>
      {#each filtered as device}
        <tr>
          <td><input class="form-check-input" type="checkbox" checked={selected.has(device.mac)} on:change={(e) => toggle(device.mac, (e.currentTarget as HTMLInputElement).checked)} /></td>
          <td>{String(device.device_num).padStart(3, '0')}</td>
          <td>{device.name || device.serial || device.mac}</td>
          <td>{device.ip}</td>
          <td><span class={`badge ${device.gen === 1 ? 'bg-danger' : device.gen === 2 ? 'bg-warning text-dark' : 'bg-success'}`}>Gen {device.gen}</span></td>
          <td>{device.model}</td>
          <td>{device.fw} {#if device.fw_available_ver}<span class="badge bg-info text-dark">↑ {device.fw_available_ver}</span>{/if}</td>
          <td><span class={`badge ${device.online ? 'bg-success' : 'bg-secondary'}`}>{device.online ? 'Online' : 'Offline'}</span></td>
          <td>{device.mqtt_enabled === null ? 'n/a' : device.mqtt_enabled ? 'On' : 'Off'}</td>
          <td>{device.cloud_connected ? 'Connected' : 'Off'}</td>
          <td>{device.tz || 'n/a'}</td>
          <td><ComplianceBadge compliant={device.compliant} issues={device.compliance_issues} /></td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

{#if !loading && !error && filtered.length === 0}
  <div class="alert alert-secondary mt-3 mb-0">No devices loaded yet. Start a scan or refresh this page.</div>
{/if}
