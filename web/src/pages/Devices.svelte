<script lang="ts">
  import { onMount } from 'svelte';
  import { api, toErrorDetails, toErrorMessage } from '../lib/api';
  import { devices, navigate, refreshInterval } from '../lib/stores';
  import type { AppSettings, Device } from '../lib/types';
  import { compareDevices } from '../lib/deviceFormatters';
  import ErrorNotice from '../components/ErrorNotice.svelte';
  import ColumnPicker from './devices/ColumnPicker.svelte';
  import DevicesToolbar from './devices/DevicesToolbar.svelte';
  import DeviceTable from './devices/DeviceTable.svelte';

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
    error = toErrorMessage(err);
    errorDetails = toErrorDetails(err);
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

  $: filtered = $devices.filter((d: Device) => {
    const haystack = `${d.name} ${d.ip} ${d.mac} ${d.model} ${d.serial}`.toLowerCase();
    return haystack.includes(filter.toLowerCase());
  });

  $: onlineCount = $devices.filter((device) => device.online).length;

  $: sorted = [...filtered].sort((a, b) => {
    const result = compareDevices(a, b, sortKey);
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

<DevicesToolbar
  bind:filter
  {onlineCount}
  {loading}
  listedCount={sorted.length}
  {showColumns}
  onToggleColumns={() => (showColumns = !showColumns)}
  onRefresh={() => load(true)}
  onRebootAll={rebootAll}
/>

{#if showColumns}
  <ColumnPicker />
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

<DeviceTable
  devices={sorted}
  {sortKey}
  {sortDir}
  {setSort}
  {appSettings}
  {rowBusy}
  showEmpty={!loading && !error && sorted.length === 0}
  onRefresh={refreshOne}
  onRemove={removeOne}
  onReboot={rebootOne}
  onOpenDetail={openDetail}
/>

<!-- Styles for the extracted children live in their respective components:
     DevicesToolbar.svelte (page-hero + toolbar-search + toolbar-select +
     media query), ColumnPicker.svelte (control-panel), and
     DeviceTable.svelte (the :global tr.device-stale rules). -->
