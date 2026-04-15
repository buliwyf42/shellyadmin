<script lang="ts">
  import { onMount } from 'svelte'
  import { APIError, api } from '../lib/api'
  import { colVis, deviceColumns, devices, navigate, refreshInterval } from '../lib/stores'
  import { formatDateTime, formatRelativeDateTime } from '../lib/time'
  import type { Device } from '../lib/types'
  import ComplianceBadge from '../components/ComplianceBadge.svelte'
  import ErrorNotice from '../components/ErrorNotice.svelte'

  let filter = ''
  let loading = false
  let error = ''
  let errorDetails = ''
  let showColumns = false
  let sortKey = 'device_num'
  let sortDir: 'asc' | 'desc' = 'asc'
  let autoRefreshTimer: ReturnType<typeof setInterval> | null = null
  let rowBusy: Record<string, { refresh: boolean; remove: boolean }> = {}

  function captureError(err: unknown) {
    if (err instanceof APIError) {
      error = err.message
      errorDetails = `${err.method} ${err.path} -> ${err.status}\n${JSON.stringify(err.detail ?? {}, null, 2)}`
      return
    }
    error = (err as Error).message
    errorDetails = String(err)
  }

  async function load(refresh = false) {
    loading = true
    error = ''
    errorDetails = ''
    try {
      $devices = refresh ? await api.refreshDevices() : await api.getDevices()
    } catch (err) {
      captureError(err)
    } finally {
      loading = false
    }
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

  async function refreshOne(device: Device) {
    error = ''
    errorDetails = ''
    rowBusy = { ...rowBusy, [device.mac]: { ...(rowBusy[device.mac] || { refresh: false, remove: false }), refresh: true } }
    try {
      $devices = await api.refreshDevice(device.mac)
    } catch (err) {
      captureError(err)
    } finally {
      rowBusy = { ...rowBusy, [device.mac]: { ...(rowBusy[device.mac] || { refresh: false, remove: false }), refresh: false } }
    }
  }

  async function removeOne(device: Device) {
    const label = device.name || device.ip || device.mac
    if (!confirm(`Delete device "${label}"?`)) return
    error = ''
    errorDetails = ''
    rowBusy = { ...rowBusy, [device.mac]: { ...(rowBusy[device.mac] || { refresh: false, remove: false }), remove: true } }
    try {
      await api.forgetDevice(device.mac)
      await load()
    } catch (err) {
      captureError(err)
    } finally {
      rowBusy = { ...rowBusy, [device.mac]: { ...(rowBusy[device.mac] || { refresh: false, remove: false }), remove: false } }
    }
  }

  function openDetail(device: Device) {
    navigate(`/devices/${encodeURIComponent(device.mac)}`)
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

  function supportsWebSocket(device: Device): boolean {
    return device.gen >= 2
  }

  function refreshState(device: Device): 'fresh' | 'stale' {
    return device.last_refresh_ok ? 'fresh' : 'stale'
  }

  function refreshStateBadgeClass(device: Device): string {
    return refreshState(device) === 'fresh' ? 'bg-success' : 'bg-secondary'
  }

  function refreshStateText(device: Device): string {
    return refreshState(device) === 'fresh' ? 'Fresh' : 'Stale'
  }

  function refreshStateTitle(device: Device): string {
    if (device.last_refresh_ok) {
      return `Last successful refresh: ${formatDateTime(device.last_seen)}`
    }
    const lastSuccess = device.last_seen ? formatDateTime(device.last_seen) : 'never'
    const lastAttempt = device.last_refresh_attempt ? formatDateTime(device.last_refresh_attempt) : 'unknown'
    const reason = device.last_refresh_error || 'latest refresh did not return device data'
    return `Latest refresh failed: ${reason}. Last attempt: ${lastAttempt}. Last successful refresh: ${lastSuccess}.`
  }

  function clearAutoRefresh(): void {
    if (autoRefreshTimer) {
      clearInterval(autoRefreshTimer)
      autoRefreshTimer = null
    }
  }

  function setupAutoRefresh(intervalMs: number): void {
    clearAutoRefresh()
    if (intervalMs > 0) {
      autoRefreshTimer = setInterval(() => {
        void load(true)
      }, intervalMs)
    }
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

  $: onlineCount = $devices.filter((device) => device.online).length

  $: sorted = [...filtered].sort((a, b) => {
    const result = compare(a, b, sortKey)
    return sortDir === 'asc' ? result : -result
  })

  $: setupAutoRefresh($refreshInterval)

  onMount(() => {
    void load()
    return () => clearAutoRefresh()
  })
</script>

<section class="page-hero">
  <div class="page-title-row">
    <span class="page-kicker">Devices</span>
    <span class="page-status">{onlineCount} online</span>
    {#if loading}
      <span class="page-status muted">Refreshing…</span>
    {/if}
  </div>
  <div class="page-hero-controls">
    <input class="form-control toolbar-search" placeholder="Filter name / IP / MAC / model" bind:value={filter} />
    <select class="form-select toolbar-select" bind:value={$refreshInterval}>
      <option value={0}>Auto refresh: Off</option>
      <option value={30000}>Auto refresh: 30 sec</option>
      <option value={60000}>Auto refresh: 1 min</option>
      <option value={300000}>Auto refresh: 5 min</option>
    </select>
    <button class="btn btn-outline-light" on:click={() => showColumns = !showColumns}>{showColumns ? 'Hide Columns' : 'Columns'}</button>
    <button class="btn btn-warning text-dark" on:click={() => load(true)} disabled={loading}>Refresh</button>
  </div>
</section>

{#if showColumns}
  <div class="card bg-dark border-secondary mb-3 control-panel">
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

<ErrorNotice summary={error} details={errorDetails} />

<div class="table-responsive dashboard-table-wrap">
  <table class="table table-dark table-striped align-middle table-nowrap dashboard-table">
    <thead>
      <tr>
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
        {#if $colVis.last_seen}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('last_seen')}>{sortLabel('Last Success', 'last_seen')}</button></th>{/if}
        {#if $colVis.compliance}<th><button class="btn btn-link px-0 text-decoration-none" on:click={() => setSort('compliance')}>{sortLabel('Compliance', 'compliance')}</button></th>{/if}
        <th class="text-end">Actions</th>
      </tr>
    </thead>
    <tbody>
      {#each sorted as device}
        <tr class:device-stale={refreshState(device) === 'stale'}>
          {#if $colVis.device_num}<td>{String(device.device_num).padStart(2, '0')}</td>{/if}
          {#if $colVis.name}<td>{device.name || device.serial || device.mac}</td>{/if}
          {#if $colVis.ip}<td><a href={`http://${device.ip}`} target="_blank" rel="noreferrer" class="ip-link">{device.ip}</a></td>{/if}
          {#if $colVis.mac}<td class="font-monospace">{device.mac}</td>{/if}
          {#if $colVis.gen}<td><span class={`badge ${supportClass(device)}`} title={supportTitle(device)}>{generationLabel(device)}</span></td>{/if}
          {#if $colVis.model}<td>{#if device.model}{device.model}{:else}<span class="text-secondary">n/a</span>{/if}</td>{/if}
          {#if $colVis.fw}
            <td>
              {#if device.fw}{device.fw}{:else}<span class="text-secondary">n/a</span>{/if}
              {#if device.fw_available_ver}<span class="badge bg-info text-dark">↑ {device.fw_available_ver}</span>{/if}
            </td>
          {/if}
          {#if $colVis.online}
            <td>
              <div class="d-flex gap-2 align-items-center flex-wrap">
                <span class={`badge ${statusBadgeClass(device.online)}`}>{statusText(device.online, 'Online', 'Offline')}</span>
                <span class={`badge ${refreshStateBadgeClass(device)}`} title={refreshStateTitle(device)}>{refreshStateText(device)}</span>
              </div>
            </td>
          {/if}
          {#if $colVis.wifi_ssid}<td>{#if device.wifi_ssid}{device.wifi_ssid}{:else}<span class="text-secondary">n/a</span>{/if}</td>{/if}
          {#if $colVis.mqtt_enabled}
            <td><span class={`badge ${mqttManagedByCloud(device) ? 'bg-secondary' : statusBadgeClass(device.mqtt_enabled)}`}>{mqttManagedByCloud(device) ? 'cloud-managed' : statusText(device.mqtt_enabled)}</span></td>
          {/if}
          {#if $colVis.mqtt_server}<td>{#if mqttManagedByCloud(device)}cloud-managed{:else if device.mqtt_server}{device.mqtt_server}{:else}<span class="text-secondary">n/a</span>{/if}</td>{/if}
          {#if $colVis.mqtt_client_id}<td>{#if mqttManagedByCloud(device)}cloud-managed{:else if device.mqtt_client_id}{device.mqtt_client_id}{:else}<span class="text-secondary">n/a</span>{/if}</td>{/if}
          {#if $colVis.mqtt_topic_prefix}<td>{#if mqttManagedByCloud(device)}cloud-managed{:else if device.mqtt_topic_prefix}{device.mqtt_topic_prefix}{:else}<span class="text-secondary">n/a</span>{/if}</td>{/if}
          {#if $colVis.cloud_connected}<td><span class={`badge ${statusBadgeClass(device.cloud_connected)}`}>{statusText(device.cloud_connected, 'Connected', 'Off')}</span></td>{/if}
          {#if $colVis.ws_connected}
            <td>
              {#if supportsWebSocket(device)}
                <span class={`badge ${statusBadgeClass(device.ws_connected)}`}>{statusText(device.ws_connected, 'Connected', 'Off')}</span>
              {:else}
                <span class="badge bg-secondary" title="WebSocket is not available on Gen1">🔒</span>
              {/if}
            </td>
          {/if}
          {#if $colVis.tz}<td>{#if device.tz}{device.tz}{:else}<span class="text-secondary">n/a</span>{/if}</td>{/if}
          {#if $colVis.time_format}<td>{#if device.time_format}{device.time_format}{:else}<span class="text-secondary">n/a</span>{/if}</td>{/if}
          {#if $colVis.sntp_server}<td>{#if device.sntp_server}{device.sntp_server}{:else}<span class="text-secondary">n/a</span>{/if}</td>{/if}
          {#if $colVis.serial}<td class="font-monospace">{#if device.serial}{device.serial}{:else}<span class="text-secondary">n/a</span>{/if}</td>{/if}
          {#if $colVis.matter_enabled}<td><span class={`badge ${statusBadgeClass(device.matter_enabled)}`}>{statusText(device.matter_enabled)}</span></td>{/if}
          {#if $colVis.ble_gw_enabled}<td><span class={`badge ${statusBadgeClass(device.ble_gw_enabled)}`}>{statusText(device.ble_gw_enabled)}</span></td>{/if}
          {#if $colVis.coords}<td>{#if formatCoords(device) !== 'n/a'}{formatCoords(device)}{:else}<span class="text-secondary">n/a</span>{/if}</td>{/if}
          {#if $colVis.eco_mode}<td><span class={`badge ${statusBadgeClass(device.eco_mode)}`}>{statusText(device.eco_mode)}</span></td>{/if}
          {#if $colVis.discoverable}<td><span class={`badge ${statusBadgeClass(device.discoverable)}`}>{statusText(device.discoverable)}</span></td>{/if}
          {#if $colVis.first_seen}<td title={formatDateTime(device.first_seen)}>{formatRelativeDateTime(device.first_seen)}</td>{/if}
          {#if $colVis.last_seen}
            <td title={refreshStateTitle(device)}>
              {#if device.last_seen}
                {formatRelativeDateTime(device.last_seen)}
              {:else}
                <span class="text-secondary">never</span>
              {/if}
            </td>
          {/if}
          {#if $colVis.compliance}<td><ComplianceBadge compliant={device.compliant} issues={device.compliance_issues} /></td>{/if}
          <td class="text-end">
            <div class="d-flex justify-content-end gap-2">
              <button
                class="btn btn-sm btn-outline-light row-action-btn"
                title="Open device detail"
                on:click={() => openDetail(device)}
                disabled={rowBusy[device.mac]?.refresh || rowBusy[device.mac]?.remove}
              >⋯</button>
              <button
                class="btn btn-sm btn-outline-light row-action-btn"
                title="Refresh this device now"
                on:click={() => refreshOne(device)}
                disabled={rowBusy[device.mac]?.refresh || rowBusy[device.mac]?.remove}
              >↻</button>
              <button
                class="btn btn-sm btn-outline-danger row-action-btn"
                title="Delete this device"
                on:click={() => removeOne(device)}
                disabled={rowBusy[device.mac]?.refresh || rowBusy[device.mac]?.remove}
              >🗑</button>
            </div>
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

{#if !loading && !error && sorted.length === 0}
  <div class="alert alert-secondary mt-3 mb-0">No devices loaded yet. Start a scan or refresh this page.</div>
{/if}

<style>
  :global(tr.device-stale td) {
    opacity: 0.62;
  }

  :global(tr.device-stale .badge) {
    opacity: 1;
  }

  :global(tr.device-stale .row-action-btn) {
    opacity: 1;
  }

  .page-hero {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 0.5rem;
    padding-bottom: 0.45rem;
    border-bottom: 1px solid rgba(160, 177, 190, 0.18);
  }

  .page-title-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: nowrap;
    min-width: 0;
  }

  .page-kicker {
    font-size: 0.95rem;
    font-weight: 700;
    line-height: 1;
    white-space: nowrap;
  }

  .page-status {
    color: #39c37c;
    font-size: 0.68rem;
    font-weight: 700;
    white-space: nowrap;
  }

  .page-status.muted {
    color: #d2b14e;
  }

  .page-hero-controls {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex-wrap: nowrap;
    justify-content: flex-end;
    flex: 1 1 auto;
    min-width: 0;
  }

  .toolbar-search {
    width: var(--toolbar-control-width-lg);
    flex: 0 0 var(--toolbar-control-width-lg);
  }

  .toolbar-select {
    width: 18rem;
    min-width: 18rem;
    flex: 0 0 18rem;
  }

  .page-hero-controls :global(.form-control),
  .page-hero-controls :global(.form-select),
  .page-hero-controls :global(.btn) {
    min-height: var(--control-height-sm);
    font-size: 0.76rem;
  }

  .page-hero-controls :global(.form-select) {
    padding-right: 2rem;
  }

  .page-hero-controls :global(.btn) {
    padding-left: 0.62rem;
    padding-right: 0.62rem;
    white-space: nowrap;
  }

  .control-panel :global(.card-body) {
    padding-top: 1rem;
  }

  @media (max-width: 900px) {
    .page-hero {
      flex-direction: column;
      align-items: stretch;
    }

    .page-hero-controls {
      width: 100%;
      justify-content: flex-start;
      flex-wrap: wrap;
    }

    .page-title-row {
      flex-wrap: wrap;
    }

    .toolbar-search {
      width: 100%;
      flex-basis: 100%;
    }
  }
</style>
