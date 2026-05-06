<script lang="ts">
  import { onMount } from 'svelte';
  import { APIError, api } from '../lib/api';
  import {
    colVis,
    deviceColumns,
    devices,
    firmwareChannel,
    navigate,
    refreshInterval,
  } from '../lib/stores';
  import { formatDateTime, formatRelativeDateTime } from '../lib/time';
  import type { AppSettings, Device } from '../lib/types';
  import { genBadgeClass, genLabel, genTitle } from '../lib/genBadge';
  import ComplianceBadge from '../components/ComplianceBadge.svelte';
  import ErrorNotice from '../components/ErrorNotice.svelte';
  import SortHeader from '../components/SortHeader.svelte';

  let filter = '';
  let loading = false;
  let appSettings: AppSettings | null = null;
  let error = '';
  let errorDetails = '';
  let showColumns = false;
  let sortKey = 'device_num';
  let sortDir: 'asc' | 'desc' = 'asc';
  let autoRefreshTimer: ReturnType<typeof setInterval> | null = null;
  let rowBusy: Record<string, { refresh: boolean; remove: boolean; reboot: boolean }> = {};
  let rebootNotice = '';

  function captureError(err: unknown) {
    if (err instanceof APIError) {
      error = err.message;
      errorDetails = `${err.method} ${err.path} -> ${err.status}\n${JSON.stringify(err.detail ?? {}, null, 2)}`;
      return;
    }
    error = (err as Error).message;
    errorDetails = String(err);
  }

  async function load(refresh = false) {
    loading = true;
    error = '';
    errorDetails = '';
    try {
      $devices = refresh ? await api.refreshDevices() : await api.getDevices();
    } catch (err) {
      captureError(err);
    } finally {
      loading = false;
    }
  }

  function toggleColumn(key: string, checked: boolean) {
    $colVis = { ...$colVis, [key]: checked };
  }

  function setSort(key: string) {
    if (sortKey === key) {
      sortDir = sortDir === 'asc' ? 'desc' : 'asc';
      return;
    }
    sortKey = key;
    sortDir = 'asc';
  }

  async function refreshOne(device: Device) {
    error = '';
    errorDetails = '';
    rowBusy = {
      ...rowBusy,
      [device.mac]: {
        ...(rowBusy[device.mac] || { refresh: false, remove: false, reboot: false }),
        refresh: true,
      },
    };
    try {
      $devices = await api.refreshDevice(device.mac);
    } catch (err) {
      captureError(err);
    } finally {
      rowBusy = {
        ...rowBusy,
        [device.mac]: {
          ...(rowBusy[device.mac] || { refresh: false, remove: false, reboot: false }),
          refresh: false,
        },
      };
    }
  }

  async function removeOne(device: Device) {
    const label = device.name || device.ip || device.mac;
    if (!confirm(`Delete device "${label}"?`)) return;
    error = '';
    errorDetails = '';
    rowBusy = {
      ...rowBusy,
      [device.mac]: {
        ...(rowBusy[device.mac] || { refresh: false, remove: false, reboot: false }),
        remove: true,
      },
    };
    try {
      await api.forgetDevice(device.mac);
      await load();
    } catch (err) {
      captureError(err);
    } finally {
      rowBusy = {
        ...rowBusy,
        [device.mac]: {
          ...(rowBusy[device.mac] || { refresh: false, remove: false, reboot: false }),
          remove: false,
        },
      };
    }
  }

  async function rebootOne(device: Device) {
    const label = device.name || device.ip || device.mac;
    if (!confirm(`Reboot "${label}"?\n\nThe device will be unreachable for ~20s.`)) return;
    error = '';
    errorDetails = '';
    rebootNotice = '';
    rowBusy = {
      ...rowBusy,
      [device.mac]: {
        ...(rowBusy[device.mac] || { refresh: false, remove: false, reboot: false }),
        reboot: true,
      },
    };
    try {
      const res = await api.bulk({ action: 'reboot', macs: [device.mac] });
      const r = res.results[0];
      rebootNotice =
        r?.status === 'ok'
          ? `Rebooted ${label}.`
          : `Reboot failed for ${label}: ${r?.detail ?? 'unknown error'}`;
    } catch (err) {
      captureError(err);
    } finally {
      rowBusy = {
        ...rowBusy,
        [device.mac]: {
          ...(rowBusy[device.mac] || { refresh: false, remove: false, reboot: false }),
          reboot: false,
        },
      };
    }
  }

  async function rebootAll() {
    const macs = sorted.map((d) => d.mac);
    if (!macs.length) return;
    if (
      !confirm(
        `Reboot all ${macs.length} listed device(s)?\n\nDevices will be unreachable for ~20s; active scan/refresh jobs may error.`,
      )
    )
      return;
    error = '';
    errorDetails = '';
    rebootNotice = '';
    loading = true;
    try {
      const res = await api.bulk({ action: 'reboot', macs });
      const failed = res.results.filter((r) => r.status !== 'ok');
      rebootNotice = failed.length
        ? `Rebooted ${macs.length - failed.length}/${macs.length} devices. ${failed.length} failed.`
        : `Rebooted ${macs.length} device(s).`;
    } catch (err) {
      captureError(err);
    } finally {
      loading = false;
    }
  }

  function openDetail(device: Device) {
    navigate(`/devices/${encodeURIComponent(device.mac)}`);
  }

  function generationLabel(device: Device): string {
    return genLabel(device.gen);
  }

  function supportClass(device: Device): string {
    return genBadgeClass(device.gen, appSettings);
  }

  function supportTitle(device: Device): string {
    return genTitle(device.gen);
  }

  function statusBadgeClass(
    value: boolean | null | undefined,
    positive = 'bg-success',
    negative = 'bg-danger',
    unknown = 'bg-secondary',
  ) {
    if (value === null || value === undefined) return 'bg-secondary';
    return value ? positive : negative;
  }

  function statusText(
    value: boolean | null | undefined,
    on = 'On',
    off = 'Off',
    na = 'n/a',
  ): string {
    if (value === null || value === undefined) return na;
    return value ? on : off;
  }

  function mqttManagedByCloud(_device: Device): boolean {
    return false;
  }

  function formatCoords(device: Device): string {
    if (device.lat === null || device.lon === null) return 'n/a';
    return `${device.lat.toFixed(5)}, ${device.lon.toFixed(5)}`;
  }

  function supportsWebSocket(_device: Device): boolean {
    return true;
  }

  function refreshState(device: Device): 'fresh' | 'stale' {
    return device.last_refresh_ok ? 'fresh' : 'stale';
  }

  function refreshStateBadgeClass(device: Device): string {
    return refreshState(device) === 'fresh' ? 'bg-success' : 'bg-secondary';
  }

  function refreshStateText(device: Device): string {
    return refreshState(device) === 'fresh' ? 'Fresh' : 'Stale';
  }

  function refreshStateTitle(device: Device): string {
    if (device.last_refresh_ok) {
      return `Last successful refresh: ${formatDateTime(device.last_seen)}`;
    }
    const lastSuccess = device.last_seen ? formatDateTime(device.last_seen) : 'never';
    const lastAttempt = device.last_refresh_attempt
      ? formatDateTime(device.last_refresh_attempt)
      : 'unknown';
    const reason = device.last_refresh_error || 'latest refresh did not return device data';
    return `Latest refresh failed: ${reason}. Last attempt: ${lastAttempt}. Last successful refresh: ${lastSuccess}.`;
  }

  function clearAutoRefresh(): void {
    if (autoRefreshTimer) {
      clearInterval(autoRefreshTimer);
      autoRefreshTimer = null;
    }
  }

  function setupAutoRefresh(intervalMs: number): void {
    clearAutoRefresh();
    if (intervalMs > 0) {
      autoRefreshTimer = setInterval(() => {
        void load(true);
      }, intervalMs);
    }
  }

  function compare(a: Device, b: Device, key: string): number {
    switch (key) {
      case 'device_num':
        return a.device_num - b.device_num;
      case 'name':
        return (a.name || a.serial || a.mac).localeCompare(b.name || b.serial || b.mac);
      case 'ip':
        return a.ip.localeCompare(b.ip, undefined, { numeric: true });
      case 'mac':
        return a.mac.localeCompare(b.mac);
      case 'gen':
        return a.gen - b.gen;
      case 'model':
        return (a.model || '').localeCompare(b.model || '');
      case 'fw':
        return (a.fw || '').localeCompare(b.fw || '');
      case 'online':
        return Number(a.online) - Number(b.online);
      case 'wifi_ssid':
        return (a.wifi_ssid || '').localeCompare(b.wifi_ssid || '');
      case 'mqtt_enabled':
        return Number(Boolean(a.mqtt_enabled)) - Number(Boolean(b.mqtt_enabled));
      case 'mqtt_server':
        return (a.mqtt_server || '').localeCompare(b.mqtt_server || '');
      case 'mqtt_client_id':
        return (a.mqtt_client_id || '').localeCompare(b.mqtt_client_id || '');
      case 'mqtt_topic_prefix':
        return (a.mqtt_topic_prefix || '').localeCompare(b.mqtt_topic_prefix || '');
      case 'cloud_connected':
        return Number(a.cloud_connected) - Number(b.cloud_connected);
      case 'ws_connected':
        return Number(a.ws_connected) - Number(b.ws_connected);
      case 'tz':
        return (a.tz || '').localeCompare(b.tz || '');
      case 'sntp_server':
        return (a.sntp_server || '').localeCompare(b.sntp_server || '');
      case 'serial':
        return (a.serial || '').localeCompare(b.serial || '');
      case 'matter_enabled':
        return Number(Boolean(a.matter_enabled)) - Number(Boolean(b.matter_enabled));
      case 'ble_gw_enabled':
        return Number(Boolean(a.ble_gw_enabled)) - Number(Boolean(b.ble_gw_enabled));
      case 'coords':
        return formatCoords(a).localeCompare(formatCoords(b));
      case 'eco_mode':
        return Number(Boolean(a.eco_mode)) - Number(Boolean(b.eco_mode));
      case 'discoverable':
        return Number(Boolean(a.discoverable)) - Number(Boolean(b.discoverable));
      case 'scheme':
        return (a.scheme || '').localeCompare(b.scheme || '');
      case 'wifi_hostname':
        return (a.wifi_hostname || '').localeCompare(b.wifi_hostname || '');
      case 'wifi_channel':
        return (a.wifi_channel || 0) - (b.wifi_channel || 0);
      case 'enhanced_security':
        return Number(Boolean(a.enhanced_security)) - Number(Boolean(b.enhanced_security));
      case 'tls_cert_valid':
        return Number(Boolean(a.tls_cert_valid)) - Number(Boolean(b.tls_cert_valid));
      case 'power_w':
        return (a.power_w ?? -1) - (b.power_w ?? -1);
      case 'voltage_v':
        return (a.voltage_v ?? -1) - (b.voltage_v ?? -1);
      case 'current_a':
        return (a.current_a ?? -1) - (b.current_a ?? -1);
      case 'first_seen':
        return (a.first_seen || '').localeCompare(b.first_seen || '');
      case 'last_seen':
        return (a.last_seen || '').localeCompare(b.last_seen || '');
      case 'compliance':
        return Number(a.compliant) - Number(b.compliant);
      default:
        return 0;
    }
  }

  $: filtered = $devices.filter((d: Device) => {
    const haystack = `${d.name} ${d.ip} ${d.mac} ${d.model} ${d.serial}`.toLowerCase();
    return haystack.includes(filter.toLowerCase());
  });

  $: onlineCount = $devices.filter((device) => device.online).length;

  $: sorted = [...filtered].sort((a, b) => {
    const result = compare(a, b, sortKey);
    return sortDir === 'asc' ? result : -result;
  });

  $: setupAutoRefresh($refreshInterval);

  onMount(() => {
    void load();
    api
      .getSettings()
      .then((s) => (appSettings = s))
      .catch(() => undefined);
    return () => clearAutoRefresh();
  });
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
    <input
      class="form-control toolbar-search"
      placeholder="Filter name / IP / MAC / model"
      bind:value={filter}
    />
    <select class="form-select toolbar-select" bind:value={$refreshInterval}>
      <option value={0}>Auto refresh: Off</option>
      <option value={30000}>Auto refresh: 30 sec</option>
      <option value={60000}>Auto refresh: 1 min</option>
      <option value={300000}>Auto refresh: 5 min</option>
    </select>
    <select
      class="form-select toolbar-select"
      bind:value={$firmwareChannel}
      title="Which channel's update version to highlight in the FW column"
    >
      <option value="stable">FW channel: Stable</option>
      <option value="beta">FW channel: Beta</option>
    </select>
    <button class="btn btn-outline-light" on:click={() => (showColumns = !showColumns)}
      >{showColumns ? 'Hide Columns' : 'Columns'}</button
    >
    <button class="btn btn-warning text-dark" on:click={() => load(true)} disabled={loading}
      >Refresh</button
    >
    <button
      class="btn btn-outline-warning"
      on:click={rebootAll}
      disabled={loading || sorted.length === 0}
      title="Reboot all listed devices">Reboot All</button
    >
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
                on:change={(e) =>
                  toggleColumn(column.key, (e.currentTarget as HTMLInputElement).checked)}
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

{#if rebootNotice}
  <div class="alert alert-info alert-dismissible py-2 mb-2" role="status">
    {rebootNotice}
    <button
      type="button"
      class="btn-close btn-close-white"
      aria-label="Dismiss"
      on:click={() => (rebootNotice = '')}
    ></button>
  </div>
{/if}

<div class="table-responsive dashboard-table-wrap">
  <table class="table table-dark table-striped align-middle table-nowrap dashboard-table">
    <thead>
      <tr>
        {#if $colVis.device_num}<SortHeader
            label="#"
            column="device_num"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.name}<SortHeader
            label="Name"
            column="name"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.ip}<SortHeader
            label="IP"
            column="ip"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.mac}<SortHeader
            label="MAC"
            column="mac"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.gen}<SortHeader
            label="Type"
            column="gen"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.model}<SortHeader
            label="Model"
            column="model"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.fw}<SortHeader
            label="Firmware"
            column="fw"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.fw_auto_update}<SortHeader
            label="Auto-Update"
            column="fw_auto_update"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.online}<SortHeader
            label="Online"
            column="online"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.wifi_ssid}<SortHeader
            label="WiFi"
            column="wifi_ssid"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.mqtt_enabled}<SortHeader
            label="MQTT"
            column="mqtt_enabled"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.mqtt_server}<SortHeader
            label="MQTT Server"
            column="mqtt_server"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.mqtt_client_id}<SortHeader
            label="MQTT Client ID"
            column="mqtt_client_id"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.mqtt_topic_prefix}<SortHeader
            label="MQTT Topic"
            column="mqtt_topic_prefix"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.cloud_connected}<SortHeader
            label="Cloud"
            column="cloud_connected"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.ws_connected}<SortHeader
            label="WebSocket"
            column="ws_connected"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.tz}<SortHeader
            label="Timezone"
            column="tz"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.sntp_server}<SortHeader
            label="SNTP"
            column="sntp_server"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.serial}<SortHeader
            label="Serial"
            column="serial"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.matter_enabled}<SortHeader
            label="Matter"
            column="matter_enabled"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.ble_gw_enabled}<SortHeader
            label="BLE GW"
            column="ble_gw_enabled"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.coords}<SortHeader
            label="Coords"
            column="coords"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.eco_mode}<SortHeader
            label="Eco"
            column="eco_mode"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.discoverable}<SortHeader
            label="Discoverable"
            column="discoverable"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.scheme}<SortHeader
            label="Scheme"
            column="scheme"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.wifi_hostname}<SortHeader
            label="Hostname"
            column="wifi_hostname"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.wifi_channel}<SortHeader
            label="WiFi Ch"
            column="wifi_channel"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.enhanced_security}<SortHeader
            label="Enhanced Sec"
            column="enhanced_security"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.tls_cert_valid}<SortHeader
            label="TLS OK"
            column="tls_cert_valid"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.power_w}<SortHeader
            label="Power (W)"
            column="power_w"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.voltage_v}<SortHeader
            label="Voltage (V)"
            column="voltage_v"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.current_a}<SortHeader
            label="Current (A)"
            column="current_a"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.first_seen}<SortHeader
            label="First Seen"
            column="first_seen"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.last_seen}<SortHeader
            label="Last Success"
            column="last_seen"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.compliance}<SortHeader
            label="Compliance"
            column="compliance"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        <th class="text-end">Actions</th>
      </tr>
    </thead>
    <tbody>
      {#each sorted as device}
        <tr class:device-stale={refreshState(device) === 'stale'}>
          {#if $colVis.device_num}<td>{String(device.device_num).padStart(2, '0')}</td>{/if}
          {#if $colVis.name}<td>{device.name || device.serial || device.mac}</td>{/if}
          {#if $colVis.ip}<td
              ><a href={`http://${device.ip}`} target="_blank" rel="noreferrer" class="ip-link"
                >{device.ip}</a
              ></td
            >{/if}
          {#if $colVis.mac}<td class="font-monospace">{device.mac}</td>{/if}
          {#if $colVis.gen}<td
              ><span class={`badge ${supportClass(device)}`} title={supportTitle(device)}
                >{generationLabel(device)}</span
              ></td
            >{/if}
          {#if $colVis.model}<td
              >{#if device.model}{device.model}{:else}<span class="text-secondary">n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.fw}
            <td>
              {#if device.fw}{device.fw}{:else}<span class="text-secondary">n/a</span>{/if}
              {#if $firmwareChannel === 'beta' && device.fw_available_beta && device.fw_available_beta !== device.fw}
                <span class="badge bg-info text-dark" title="beta available">
                  ↑ {device.fw_available_beta}
                </span>
              {:else if $firmwareChannel === 'stable' && device.fw_available_stable && device.fw_available_stable !== device.fw}
                <span class="badge bg-info text-dark" title="stable available">
                  ↑ {device.fw_available_stable}
                </span>
              {/if}
            </td>
          {/if}
          {#if $colVis.fw_auto_update}
            <td>
              {#if device.fw_auto_update === 'stable'}
                <span class="badge bg-success" title="auto-update Stable">stable</span>
              {:else if device.fw_auto_update === 'beta'}
                <span class="badge bg-info text-dark" title="auto-update Beta">beta</span>
              {:else if device.fw_auto_update === 'off'}
                <span class="badge bg-secondary" title="auto-update disabled">off</span>
              {:else}
                <span class="badge bg-dark border border-secondary" title="not yet read"
                  >unknown</span
                >
              {/if}
            </td>
          {/if}
          {#if $colVis.online}
            <td>
              <div class="d-flex gap-2 align-items-center flex-wrap">
                <span class={`badge ${statusBadgeClass(device.online)}`}
                  >{statusText(device.online, 'Online', 'Offline')}</span
                >
                <span
                  class={`badge ${refreshStateBadgeClass(device)}`}
                  title={refreshStateTitle(device)}>{refreshStateText(device)}</span
                >
              </div>
            </td>
          {/if}
          {#if $colVis.wifi_ssid}<td
              >{#if device.wifi_ssid}{device.wifi_ssid}{:else}<span class="text-secondary">n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.mqtt_enabled}
            <td
              ><span
                class={`badge ${mqttManagedByCloud(device) ? 'bg-secondary' : statusBadgeClass(device.mqtt_enabled)}`}
                >{mqttManagedByCloud(device)
                  ? 'cloud-managed'
                  : statusText(device.mqtt_enabled)}</span
              ></td
            >
          {/if}
          {#if $colVis.mqtt_server}<td
              >{#if mqttManagedByCloud(device)}cloud-managed{:else if device.mqtt_server}{device.mqtt_server}{:else}<span
                  class="text-secondary">n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.mqtt_client_id}<td
              >{#if mqttManagedByCloud(device)}cloud-managed{:else if device.mqtt_client_id}{device.mqtt_client_id}{:else}<span
                  class="text-secondary">n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.mqtt_topic_prefix}<td
              >{#if mqttManagedByCloud(device)}cloud-managed{:else if device.mqtt_topic_prefix}{device.mqtt_topic_prefix}{:else}<span
                  class="text-secondary">n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.cloud_connected}<td
              ><span class={`badge ${statusBadgeClass(device.cloud_connected)}`}
                >{statusText(device.cloud_connected, 'Connected', 'Off')}</span
              ></td
            >{/if}
          {#if $colVis.ws_connected}<td
              ><span class={`badge ${statusBadgeClass(device.ws_connected)}`}
                >{statusText(device.ws_connected, 'Connected', 'Off')}</span
              ></td
            >{/if}
          {#if $colVis.tz}<td
              >{#if device.tz}{device.tz}{:else}<span class="text-secondary">n/a</span>{/if}</td
            >{/if}
          {#if $colVis.sntp_server}<td
              >{#if device.sntp_server}{device.sntp_server}{:else}<span class="text-secondary"
                  >n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.serial}<td class="font-monospace"
              >{#if device.serial}{device.serial}{:else}<span class="text-secondary">n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.matter_enabled}<td
              ><span class={`badge ${statusBadgeClass(device.matter_enabled)}`}
                >{statusText(device.matter_enabled)}</span
              ></td
            >{/if}
          {#if $colVis.ble_gw_enabled}<td
              ><span class={`badge ${statusBadgeClass(device.ble_gw_enabled)}`}
                >{statusText(device.ble_gw_enabled)}</span
              ></td
            >{/if}
          {#if $colVis.coords}<td
              >{#if formatCoords(device) !== 'n/a'}{formatCoords(device)}{:else}<span
                  class="text-secondary">n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.eco_mode}<td
              ><span class={`badge ${statusBadgeClass(device.eco_mode)}`}
                >{statusText(device.eco_mode)}</span
              ></td
            >{/if}
          {#if $colVis.discoverable}<td
              ><span class={`badge ${statusBadgeClass(device.discoverable)}`}
                >{statusText(device.discoverable)}</span
              ></td
            >{/if}
          {#if $colVis.scheme}<td class="font-monospace">{device.scheme || 'http'}</td>{/if}
          {#if $colVis.wifi_hostname}<td
              >{#if device.wifi_hostname}{device.wifi_hostname}{:else}<span class="text-secondary"
                  >n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.wifi_channel}<td class="text-end"
              >{#if device.wifi_channel}{device.wifi_channel}{:else}<span class="text-secondary"
                  >n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.enhanced_security}<td
              ><span class={`badge ${statusBadgeClass(device.enhanced_security ?? null)}`}
                >{statusText(device.enhanced_security ?? null)}</span
              ></td
            >{/if}
          {#if $colVis.tls_cert_valid}<td
              ><span class={`badge ${statusBadgeClass(device.tls_cert_valid ?? null)}`}
                >{statusText(device.tls_cert_valid ?? null)}</span
              ></td
            >{/if}
          {#if $colVis.power_w}<td class="text-end font-monospace"
              >{#if device.power_w !== null && device.power_w !== undefined}{device.power_w.toFixed(
                  1,
                )}{:else}<span class="text-secondary">n/a</span>{/if}</td
            >{/if}
          {#if $colVis.voltage_v}<td class="text-end font-monospace"
              >{#if device.voltage_v !== null && device.voltage_v !== undefined}{device.voltage_v.toFixed(
                  0,
                )}{:else}<span class="text-secondary">n/a</span>{/if}</td
            >{/if}
          {#if $colVis.current_a}<td class="text-end font-monospace"
              >{#if device.current_a !== null && device.current_a !== undefined}{device.current_a.toFixed(
                  2,
                )}{:else}<span class="text-secondary">n/a</span>{/if}</td
            >{/if}
          {#if $colVis.first_seen}<td title={formatDateTime(device.first_seen)}
              >{formatRelativeDateTime(device.first_seen)}</td
            >{/if}
          {#if $colVis.last_seen}
            <td title={refreshStateTitle(device)}>
              {#if device.last_seen}
                {formatRelativeDateTime(device.last_seen)}
              {:else}
                <span class="text-secondary">never</span>
              {/if}
            </td>
          {/if}
          {#if $colVis.compliance}<td
              ><ComplianceBadge
                compliant={device.compliant}
                issues={device.compliance_issues}
              /></td
            >{/if}
          <td class="text-end">
            <div class="d-flex justify-content-end gap-2">
              <button
                class="btn btn-sm btn-outline-light row-action-btn"
                title="Open device detail"
                aria-label={`Open detail for ${device.name || device.mac}`}
                on:click={() => openDetail(device)}
                disabled={rowBusy[device.mac]?.refresh || rowBusy[device.mac]?.remove}
                ><span aria-hidden="true">⋯</span></button
              >
              <button
                class="btn btn-sm btn-outline-light row-action-btn"
                title="Refresh this device now"
                aria-label={`Refresh ${device.name || device.mac}`}
                on:click={() => refreshOne(device)}
                disabled={rowBusy[device.mac]?.refresh || rowBusy[device.mac]?.remove}
                ><span aria-hidden="true">↻</span></button
              >
              <button
                class="btn btn-sm btn-outline-warning row-action-btn"
                title="Reboot this device"
                aria-label={`Reboot ${device.name || device.mac}`}
                on:click={() => rebootOne(device)}
                disabled={rowBusy[device.mac]?.refresh ||
                  rowBusy[device.mac]?.remove ||
                  rowBusy[device.mac]?.reboot}
                >{#if rowBusy[device.mac]?.reboot}<span
                    class="spinner-border spinner-border-sm"
                    aria-hidden="true"
                  ></span>{:else}<span aria-hidden="true">⏻</span>{/if}</button
              >
              <button
                class="btn btn-sm btn-outline-danger row-action-btn"
                title="Delete this device"
                aria-label={`Delete ${device.name || device.mac}`}
                on:click={() => removeOne(device)}
                disabled={rowBusy[device.mac]?.refresh ||
                  rowBusy[device.mac]?.remove ||
                  rowBusy[device.mac]?.reboot}><span aria-hidden="true">🗑</span></button
              >
            </div>
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

{#if !loading && !error && sorted.length === 0}
  <div class="alert alert-secondary mt-3 mb-0">
    No devices loaded yet. Start a scan or refresh this page.
  </div>
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
