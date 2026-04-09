<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import { colVis, deviceColumns, devices } from '../lib/stores'
  import type { Device } from '../lib/types'
  import ComplianceBadge from '../components/ComplianceBadge.svelte'

  let filter = ''
  let selected = new Set<string>()
  let loading = false
  let error = ''
  let bulkAction = 'set_24h'
  let bulkValue = ''
  let bulkEnabled = true
  let showColumns = false
  let sortKey = 'device_num'
  let sortDir: 'asc' | 'desc' = 'asc'

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

  function toggleColumn(key: string, checked: boolean) {
    $colVis = { ...$colVis, [key]: checked }
  }

  function setSort(key: string) {
    if (sortKey === key) {
      sortDir = sortDir === 'asc' ? 'desc' : 'asc'
      return
    }
    sortKey = key
    sortDir = 'asc'
  }

  function sortLabel(label: string, key: string): string {
    if (sortKey !== key) return label
    return `${label} ${sortDir === 'asc' ? '▲' : '▼'}`
  }

  function generationLabel(device: Device): string {
    if (device.gen <= 1) return 'Gen 1.x'
    return `Gen ${device.gen}.x`
  }

  function supportClass(device: Device): string {
    if (device.gen <= 1) return 'bg-danger'
    if (device.gen === 2) return 'bg-warning text-dark'
    return 'bg-success'
  }

  function supportTitle(device: Device): string {
    if (device.gen <= 1) return 'Legacy support'
    if (device.gen === 2) return 'Limited support'
    return 'Supported'
  }

  function statusBadgeClass(value: boolean | null | undefined, positive = 'bg-success', negative = 'bg-danger', unknown = 'bg-secondary') {
    if (value === null || value === undefined) return 'bg-secondary'
    return value ? positive : negative
  }

  function statusText(value: boolean | null | undefined, on = 'On', off = 'Off', na = 'n/a'): string {
    if (value === null || value === undefined) return na
    return value ? on : off
  }

  function mqttManagedByCloud(device: Device): boolean {
    return device.gen <= 1 && device.cloud_connected
  }

  function formatCoords(device: Device): string {
    if (device.lat === null || device.lon === null) return 'n/a'
    return `${device.lat.toFixed(5)}, ${device.lon.toFixed(5)}`
  }

  function formatDate(value: string): string {
    return value ? new Date(value).toLocaleString() : 'n/a'
  }

  function compare(a: Device, b: Device, key: string): number {
    switch (key) {
      case 'device_num':
        return a.device_num - b.device_num
      case 'name':
        return (a.name || a.serial || a.mac).localeCompare(b.name || b.serial || b.mac)
      case 'ip':
        return a.ip.localeCompare(b.ip, undefined, { numeric: true })
      case 'mac':
        return a.mac.localeCompare(b.mac)
      case 'gen':
        return a.gen - b.gen
      case 'model':
        return (a.model || '').localeCompare(b.model || '')
      case 'fw':
        return (a.fw || '').localeCompare(b.fw || '')
      case 'online':
        return Number(a.online) - Number(b.online)
      case 'wifi_ssid':
        return (a.wifi_ssid || '').localeCompare(b.wifi_ssid || '')
      case 'mqtt_enabled':
        return Number(Boolean(a.mqtt_enabled)) - Number(Boolean(b.mqtt_enabled))
      case 'mqtt_server':
        return (a.mqtt_server || '').localeCompare(b.mqtt_server || '')
      case 'mqtt_client_id':
        return (a.mqtt_client_id || '').localeCompare(b.mqtt_client_id || '')
      case 'mqtt_topic_prefix':
        return (a.mqtt_topic_prefix || '').localeCompare(b.mqtt_topic_prefix || '')
      case 'cloud_connected':
        return Number(a.cloud_connected) - Number(b.cloud_connected)
      case 'ws_connected':
        return Number(a.ws_connected) - Number(b.ws_connected)
      case 'tz':
        return (a.tz || '').localeCompare(b.tz || '')
      case 'time_format':
        return (a.time_format || '').localeCompare(b.time_format || '')
      case 'sntp_server':
        return (a.sntp_server || '').localeCompare(b.sntp_server || '')
      case 'serial':
        return (a.serial || '').localeCompare(b.serial || '')
      case 'matter_enabled':
        return Number(Boolean(a.matter_enabled)) - Number(Boolean(b.matter_enabled))
      case 'ble_gw_enabled':
        return Number(Boolean(a.ble_gw_enabled)) - Number(Boolean(b.ble_gw_enabled))
      case 'coords':
        return formatCoords(a).localeCompare(formatCoords(b))
      case 'eco_mode':
        return Number(Boolean(a.eco_mode)) - Number(Boolean(b.eco_mode))
      case 'discoverable':
        return Number(Boolean(a.discoverable)) - Number(Boolean(b.discoverable))
      case 'first_seen':
        return (a.first_seen || '').localeCompare(b.first_seen || '')
      case 'last_seen':
        return (a.last_seen || '').localeCompare(b.last_seen || '')
      case 'compliance':
        return Number(a.compliant) - Number(b.compliant)
      default:
        return 0
    }
  }

  $: filtered = $devices.filter((d: Device) => {
    const haystack = `${d.name} ${d.ip} ${d.mac} ${d.model} ${d.serial}`.toLowerCase()
    return haystack.includes(filter.toLowerCase())
  })

  $: sorted = [...filtered].sort((a, b) => {
    const result = compare(a, b, sortKey)
    return sortDir === 'asc' ? result : -result
  })

  onMount(() => void load())
</script>

<div class="d-flex justify-content-between align-items-center mb-3 gap-3 flex-wrap">
  <div class="d-flex gap-2 flex-wrap">
    <button class="btn btn-warning text-dark" on:click={() => load(true)} disabled={loading}>Refresh</button>
    <button class="btn btn-outline-light" on:click={() => showColumns = !showColumns}>{showColumns ? 'Hide Columns' : 'Columns'}</button>
    <input class="form-control" placeholder="Filter name / IP / MAC / model" bind:value={filter} style="width: 20rem" />
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

{#if showColumns}
  <div class="card bg-dark border-secondary mb-3">
    <div class="card-body">
      <h2 class="h5">Visible Columns</h2>
      <div class="row g-3">
        {#each deviceColumns as column}
          <div class="col-md-4">
            <label class="d-flex align-items-center gap-2">
              <input
                class="form-check-input"
                type="checkbox"
                checked={$colVis[column.key] ?? false}
                on:change={(e) => toggleColumn(column.key, (e.currentTarget as HTMLInputElement).checked)}
              />
              <span>{column.label}</span>
            </label>
          </div>
        {/each}
      </div>
    </div>
  </div>
{/if}

{#if error}
  <div class="alert alert-danger">{error}</div>
{/if}

<div class="table-responsive">
  <table class="table table-dark table-striped align-middle table-nowrap">
    <thead>
      <tr>
        <th></th>
        {#if $colVis.device_num}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('device_num')}>{sortLabel('#', 'device_num')}</button></th>{/if}
        {#if $colVis.name}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('name')}>{sortLabel('Name', 'name')}</button></th>{/if}
        {#if $colVis.ip}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('ip')}>{sortLabel('IP', 'ip')}</button></th>{/if}
        {#if $colVis.mac}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('mac')}>{sortLabel('MAC', 'mac')}</button></th>{/if}
        {#if $colVis.gen}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('gen')}>{sortLabel('Type', 'gen')}</button></th>{/if}
        {#if $colVis.model}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('model')}>{sortLabel('Model', 'model')}</button></th>{/if}
        {#if $colVis.fw}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('fw')}>{sortLabel('Firmware', 'fw')}</button></th>{/if}
        {#if $colVis.online}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('online')}>{sortLabel('Online', 'online')}</button></th>{/if}
        {#if $colVis.wifi_ssid}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('wifi_ssid')}>{sortLabel('WiFi', 'wifi_ssid')}</button></th>{/if}
        {#if $colVis.mqtt_enabled}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('mqtt_enabled')}>{sortLabel('MQTT', 'mqtt_enabled')}</button></th>{/if}
        {#if $colVis.mqtt_server}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('mqtt_server')}>{sortLabel('MQTT Server', 'mqtt_server')}</button></th>{/if}
        {#if $colVis.mqtt_client_id}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('mqtt_client_id')}>{sortLabel('MQTT Client ID', 'mqtt_client_id')}</button></th>{/if}
        {#if $colVis.mqtt_topic_prefix}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('mqtt_topic_prefix')}>{sortLabel('MQTT Topic', 'mqtt_topic_prefix')}</button></th>{/if}
        {#if $colVis.cloud_connected}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('cloud_connected')}>{sortLabel('Cloud', 'cloud_connected')}</button></th>{/if}
        {#if $colVis.ws_connected}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('ws_connected')}>{sortLabel('WebSocket', 'ws_connected')}</button></th>{/if}
        {#if $colVis.tz}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('tz')}>{sortLabel('Timezone', 'tz')}</button></th>{/if}
        {#if $colVis.time_format}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('time_format')}>{sortLabel('Time Format', 'time_format')}</button></th>{/if}
        {#if $colVis.sntp_server}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('sntp_server')}>{sortLabel('SNTP', 'sntp_server')}</button></th>{/if}
        {#if $colVis.serial}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('serial')}>{sortLabel('Serial', 'serial')}</button></th>{/if}
        {#if $colVis.matter_enabled}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('matter_enabled')}>{sortLabel('Matter', 'matter_enabled')}</button></th>{/if}
        {#if $colVis.ble_gw_enabled}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('ble_gw_enabled')}>{sortLabel('BLE GW', 'ble_gw_enabled')}</button></th>{/if}
        {#if $colVis.coords}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('coords')}>{sortLabel('Coords', 'coords')}</button></th>{/if}
        {#if $colVis.eco_mode}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('eco_mode')}>{sortLabel('Eco', 'eco_mode')}</button></th>{/if}
        {#if $colVis.discoverable}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('discoverable')}>{sortLabel('Discoverable', 'discoverable')}</button></th>{/if}
        {#if $colVis.first_seen}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('first_seen')}>{sortLabel('First Seen', 'first_seen')}</button></th>{/if}
        {#if $colVis.last_seen}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('last_seen')}>{sortLabel('Last Seen', 'last_seen')}</button></th>{/if}
        {#if $colVis.compliance}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('compliance')}>{sortLabel('Compliance', 'compliance')}</button></th>{/if}
      </tr>
    </thead>
    <tbody>
      {#each sorted as device}
        <tr>
          <td><input class="form-check-input" type="checkbox" checked={selected.has(device.mac)} on:change={(e) => toggle(device.mac, (e.currentTarget as HTMLInputElement).checked)} /></td>
          {#if $colVis.device_num}<td>{String(device.device_num).padStart(3, '0')}</td>{/if}
          {#if $colVis.name}<td>{device.name || device.serial || device.mac}</td>{/if}
          {#if $colVis.ip}<td><a href={`http://${device.ip}`} target="_blank" rel="noreferrer" class="text-decoration-none">{device.ip}</a></td>{/if}
          {#if $colVis.mac}<td class="font-monospace">{device.mac}</td>{/if}
          {#if $colVis.gen}<td><span class={`badge ${supportClass(device)}`} title={supportTitle(device)}>{generationLabel(device)}</span></td>{/if}
          {#if $colVis.model}<td>{device.model || 'n/a'}</td>{/if}
          {#if $colVis.fw}<td>{device.fw || 'n/a'} {#if device.fw_available_ver}<span class="badge bg-info text-dark">↑ {device.fw_available_ver}</span>{/if}</td>{/if}
          {#if $colVis.online}<td><span class={`badge ${statusBadgeClass(device.online)}`}>{statusText(device.online, 'Online', 'Offline')}</span></td>{/if}
          {#if $colVis.wifi_ssid}<td>{device.wifi_ssid || 'n/a'}</td>{/if}
          {#if $colVis.mqtt_enabled}
            <td><span class={`badge ${mqttManagedByCloud(device) ? 'bg-secondary' : statusBadgeClass(device.mqtt_enabled)}`}>{mqttManagedByCloud(device) ? 'cloud-managed' : statusText(device.mqtt_enabled)}</span></td>
          {/if}
          {#if $colVis.mqtt_server}<td>{mqttManagedByCloud(device) ? 'cloud-managed' : device.mqtt_server || 'n/a'}</td>{/if}
          {#if $colVis.mqtt_client_id}<td>{mqttManagedByCloud(device) ? 'cloud-managed' : device.mqtt_client_id || 'n/a'}</td>{/if}
          {#if $colVis.mqtt_topic_prefix}<td>{mqttManagedByCloud(device) ? 'cloud-managed' : device.mqtt_topic_prefix || 'n/a'}</td>{/if}
          {#if $colVis.cloud_connected}<td><span class={`badge ${statusBadgeClass(device.cloud_connected)}`}>{statusText(device.cloud_connected, 'Connected', 'Off')}</span></td>{/if}
          {#if $colVis.ws_connected}<td><span class={`badge ${statusBadgeClass(device.ws_connected)}`}>{statusText(device.ws_connected, 'Connected', 'Off')}</span></td>{/if}
          {#if $colVis.tz}<td>{device.tz || 'n/a'}</td>{/if}
          {#if $colVis.time_format}<td>{device.time_format || 'n/a'}</td>{/if}
          {#if $colVis.sntp_server}<td>{device.sntp_server || 'n/a'}</td>{/if}
          {#if $colVis.serial}<td class="font-monospace">{device.serial || 'n/a'}</td>{/if}
          {#if $colVis.matter_enabled}<td><span class={`badge ${statusBadgeClass(device.matter_enabled)}`}>{statusText(device.matter_enabled)}</span></td>{/if}
          {#if $colVis.ble_gw_enabled}<td><span class={`badge ${statusBadgeClass(device.ble_gw_enabled)}`}>{statusText(device.ble_gw_enabled)}</span></td>{/if}
          {#if $colVis.coords}<td>{formatCoords(device)}</td>{/if}
          {#if $colVis.eco_mode}<td><span class={`badge ${statusBadgeClass(device.eco_mode)}`}>{statusText(device.eco_mode)}</span></td>{/if}
          {#if $colVis.discoverable}<td><span class={`badge ${statusBadgeClass(device.discoverable)}`}>{statusText(device.discoverable)}</span></td>{/if}
          {#if $colVis.first_seen}<td>{formatDate(device.first_seen)}</td>{/if}
          {#if $colVis.last_seen}<td>{formatDate(device.last_seen)}</td>{/if}
          {#if $colVis.compliance}<td><ComplianceBadge compliant={device.compliant} issues={device.compliance_issues} /></td>{/if}
        </tr>
      {/each}
    </tbody>
  </table>
</div>

{#if !loading && !error && sorted.length === 0}
  <div class="alert alert-secondary mt-3 mb-0">No devices loaded yet. Start a scan or refresh this page.</div>
{/if}
