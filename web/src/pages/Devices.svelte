<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import { colVis, devices } from '../lib/stores'
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

  function supportLabel(device: Device): string {
    if (device.gen <= 1) return 'Legacy'
    if (device.gen === 2) return 'Limited'
    return 'Supported'
  }

  function supportClass(device: Device): string {
    if (device.gen <= 1) return 'bg-danger'
    if (device.gen === 2) return 'bg-warning text-dark'
    return 'bg-success'
  }

  function boolText(value: boolean | null | undefined, on = 'On', off = 'Off', na = 'n/a'): string {
    if (value === null || value === undefined) return na
    return value ? on : off
  }

  function formatCoords(device: Device): string {
    if (device.lat === null || device.lon === null) return 'n/a'
    return `${device.lat.toFixed(5)}, ${device.lon.toFixed(5)}`
  }

  function formatDate(value: string): string {
    return value ? new Date(value).toLocaleString() : 'n/a'
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
        {#if $colVis.device_num}<th>#</th>{/if}
        {#if $colVis.name}<th>Name</th>{/if}
        {#if $colVis.ip}<th>IP</th>{/if}
        {#if $colVis.mac}<th>MAC</th>{/if}
        {#if $colVis.gen}<th>Support</th>{/if}
        {#if $colVis.model}<th>Model</th>{/if}
        {#if $colVis.fw}<th>Firmware</th>{/if}
        {#if $colVis.online}<th>Online</th>{/if}
        {#if $colVis.wifi_ssid}<th>WiFi</th>{/if}
        {#if $colVis.mqtt_enabled}<th>MQTT</th>{/if}
        {#if $colVis.mqtt_server}<th>MQTT Server</th>{/if}
        {#if $colVis.mqtt_client_id}<th>MQTT Client ID</th>{/if}
        {#if $colVis.mqtt_topic_prefix}<th>MQTT Topic</th>{/if}
        {#if $colVis.cloud_connected}<th>Cloud</th>{/if}
        {#if $colVis.ws_connected}<th>WebSocket</th>{/if}
        {#if $colVis.tz}<th>Timezone</th>{/if}
        {#if $colVis.time_format}<th>Time Format</th>{/if}
        {#if $colVis.sntp_server}<th>SNTP</th>{/if}
        {#if $colVis.serial}<th>Serial</th>{/if}
        {#if $colVis.matter_enabled}<th>Matter</th>{/if}
        {#if $colVis.ble_gw_enabled}<th>BLE GW</th>{/if}
        {#if $colVis.coords}<th>Coords</th>{/if}
        {#if $colVis.eco_mode}<th>Eco</th>{/if}
        {#if $colVis.discoverable}<th>Discoverable</th>{/if}
        {#if $colVis.first_seen}<th>First Seen</th>{/if}
        {#if $colVis.last_seen}<th>Last Seen</th>{/if}
        {#if $colVis.compliance}<th>Compliance</th>{/if}
      </tr>
    </thead>
    <tbody>
      {#each filtered as device}
        <tr>
          <td><input class="form-check-input" type="checkbox" checked={selected.has(device.mac)} on:change={(e) => toggle(device.mac, (e.currentTarget as HTMLInputElement).checked)} /></td>
          {#if $colVis.device_num}<td>{String(device.device_num).padStart(3, '0')}</td>{/if}
          {#if $colVis.name}<td>{device.name || device.serial || device.mac}</td>{/if}
          {#if $colVis.ip}<td>{device.ip}</td>{/if}
          {#if $colVis.mac}<td class="font-monospace">{device.mac}</td>{/if}
          {#if $colVis.gen}<td><span class={`badge ${supportClass(device)}`}>{supportLabel(device)}</span></td>{/if}
          {#if $colVis.model}<td>{device.model || 'n/a'}</td>{/if}
          {#if $colVis.fw}<td>{device.fw || 'n/a'} {#if device.fw_available_ver}<span class="badge bg-info text-dark">↑ {device.fw_available_ver}</span>{/if}</td>{/if}
          {#if $colVis.online}<td><span class={`badge ${device.online ? 'bg-success' : 'bg-secondary'}`}>{device.online ? 'Online' : 'Offline'}</span></td>{/if}
          {#if $colVis.wifi_ssid}<td>{device.wifi_ssid || 'n/a'}</td>{/if}
          {#if $colVis.mqtt_enabled}<td>{boolText(device.gen <= 1 && device.cloud_connected ? null : device.mqtt_enabled)}</td>{/if}
          {#if $colVis.mqtt_server}<td>{device.gen <= 1 && device.cloud_connected ? 'cloud-managed' : device.mqtt_server || 'n/a'}</td>{/if}
          {#if $colVis.mqtt_client_id}<td>{device.gen <= 1 && device.cloud_connected ? 'cloud-managed' : device.mqtt_client_id || 'n/a'}</td>{/if}
          {#if $colVis.mqtt_topic_prefix}<td>{device.gen <= 1 && device.cloud_connected ? 'cloud-managed' : device.mqtt_topic_prefix || 'n/a'}</td>{/if}
          {#if $colVis.cloud_connected}<td>{device.cloud_connected ? 'Connected' : 'Off'}</td>{/if}
          {#if $colVis.ws_connected}<td>{boolText(device.ws_connected, 'Connected', 'Off')}</td>{/if}
          {#if $colVis.tz}<td>{device.tz || 'n/a'}</td>{/if}
          {#if $colVis.time_format}<td>{device.time_format || 'n/a'}</td>{/if}
          {#if $colVis.sntp_server}<td>{device.sntp_server || 'n/a'}</td>{/if}
          {#if $colVis.serial}<td class="font-monospace">{device.serial || 'n/a'}</td>{/if}
          {#if $colVis.matter_enabled}<td>{boolText(device.matter_enabled)}</td>{/if}
          {#if $colVis.ble_gw_enabled}<td>{boolText(device.ble_gw_enabled)}</td>{/if}
          {#if $colVis.coords}<td>{formatCoords(device)}</td>{/if}
          {#if $colVis.eco_mode}<td>{boolText(device.eco_mode)}</td>{/if}
          {#if $colVis.discoverable}<td>{boolText(device.discoverable)}</td>{/if}
          {#if $colVis.first_seen}<td>{formatDate(device.first_seen)}</td>{/if}
          {#if $colVis.last_seen}<td>{formatDate(device.last_seen)}</td>{/if}
          {#if $colVis.compliance}<td><ComplianceBadge compliant={device.compliant} issues={device.compliance_issues} /></td>{/if}
        </tr>
      {/each}
    </tbody>
  </table>
</div>

{#if !loading && !error && filtered.length === 0}
  <div class="alert alert-secondary mt-3 mb-0">No devices loaded yet. Start a scan or refresh this page.</div>
{/if}
