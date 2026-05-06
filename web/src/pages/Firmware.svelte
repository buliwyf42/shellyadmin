<script lang="ts">
  import { onDestroy } from 'svelte';
  import { APIError, api } from '../lib/api';
  import type {
    Device,
    FWResult,
    FirmwareInstallResult,
    FirmwareInstallStatus,
    FirmwareStatus,
  } from '../lib/types';
  import ErrorNotice from '../components/ErrorNotice.svelte';
  import ProgressBar from '../components/ProgressBar.svelte';

  type Channel = 'stable' | 'beta';

  let stage: Channel = 'stable';
  let checkStatus: FirmwareStatus = { running: false, done: 0, total: 0, results: [] };
  let installStatus: FirmwareInstallStatus = { running: false, done: 0, total: 0, results: [] };
  let devicesByMAC: Record<string, Device> = {};
  let selectedMacs: string[] = [];
  let checkTimer: number | undefined;
  let installTimer: number | undefined;
  let confirmOpen = false;
  let error = '';
  let errorDetails = '';

  function captureError(err: unknown) {
    if (err instanceof APIError) {
      error = err.message;
      errorDetails = `${err.method} ${err.path} -> ${err.status}\n${JSON.stringify(err.detail ?? {}, null, 2)}`;
      return;
    }
    error = (err as Error).message;
    errorDetails = String(err);
  }

  async function refreshDeviceIndex() {
    try {
      const list = await api.getDevices();
      const next: Record<string, Device> = {};
      for (const d of list) next[d.mac] = d;
      devicesByMAC = next;
    } catch (err) {
      captureError(err);
    }
  }

  refreshDeviceIndex();
  // Pull the latest persisted check + install state on mount so the page
  // doesn't render empty when the user navigates back to it mid-job.
  api.firmwareStatus().then((s) => {
    checkStatus = s;
    if (s.running) startCheckPolling();
  });
  api.firmwareInstallStatus().then((s) => {
    installStatus = s;
    if (s.running) startInstallPolling();
  });

  function startCheckPolling() {
    if (checkTimer) return;
    checkTimer = window.setInterval(async () => {
      try {
        checkStatus = await api.firmwareStatus();
        if (!checkStatus.running) {
          stopCheckPolling();
          // Re-pull devices so Available columns reflect freshly persisted cache.
          refreshDeviceIndex();
        }
      } catch (err) {
        captureError(err);
        stopCheckPolling();
      }
    }, 2000);
  }

  function stopCheckPolling() {
    if (checkTimer) {
      clearInterval(checkTimer);
      checkTimer = undefined;
    }
  }

  function startInstallPolling() {
    if (installTimer) return;
    installTimer = window.setInterval(async () => {
      try {
        installStatus = await api.firmwareInstallStatus();
        if (!installStatus.running) {
          stopInstallPolling();
          refreshDeviceIndex();
        }
      } catch (err) {
        captureError(err);
        stopInstallPolling();
      }
    }, 2000);
  }

  function stopInstallPolling() {
    if (installTimer) {
      clearInterval(installTimer);
      installTimer = undefined;
    }
  }

  onDestroy(() => {
    stopCheckPolling();
    stopInstallPolling();
  });

  async function check() {
    error = '';
    errorDetails = '';
    selectedMacs = [];
    try {
      await api.firmwareCheck();
      checkStatus = { ...checkStatus, running: true };
      startCheckPolling();
    } catch (err) {
      captureError(err);
    }
  }

  async function confirmAndUpdate() {
    confirmOpen = false;
    error = '';
    errorDetails = '';
    try {
      await api.firmwareUpdate(selectedMacs, stage);
      installStatus = { ...installStatus, running: true };
      startInstallPolling();
    } catch (err) {
      captureError(err);
    }
  }

  function openConfirm() {
    if (selectedMacs.length === 0) return;
    confirmOpen = true;
  }

  // Merge: index check results by MAC, install results by MAC.
  $: checkByMAC = (() => {
    const m: Record<string, FWResult> = {};
    for (const r of checkStatus.results) m[r.mac] = r;
    return m;
  })();

  $: installByMAC = (() => {
    const m: Record<string, FirmwareInstallResult> = {};
    for (const r of installStatus.results) m[r.mac] = r;
    return m;
  })();

  // Build the list of rows to render. The Update page is fundamentally about
  // the persisted device inventory + the per-channel cache on each row, so we
  // iterate devices and overlay the latest check / install state.
  $: rows = Object.values(devicesByMAC)
    .slice()
    .sort((a, b) => a.device_num - b.device_num)
    .map((d) => {
      const c = checkByMAC[d.mac];
      const i = installByMAC[d.mac];
      const stableVer = c ? c.stable_ver : d.fw_available_stable;
      const betaVer = c ? c.beta_ver : d.fw_available_beta;
      const checkErr = c?.status === 'error' ? c.note : '';
      return {
        mac: d.mac,
        ip: d.ip,
        name: d.name,
        model: d.model,
        currentVer: d.fw,
        stableVer,
        betaVer,
        stableUpdate: stableVer !== '' && stableVer !== d.fw,
        betaUpdate: betaVer !== '' && betaVer !== d.fw,
        installState: i,
        checkErr,
      };
    });

  function hasUpdateOnChannel(row: { stableUpdate: boolean; betaUpdate: boolean }, ch: Channel) {
    return ch === 'beta' ? row.betaUpdate : row.stableUpdate;
  }

  // Auto-uncheck rows that don't have an update on the active channel.
  $: {
    const valid = new Set(rows.filter((r) => hasUpdateOnChannel(r, stage)).map((r) => r.mac));
    const filtered = selectedMacs.filter((m) => valid.has(m));
    if (filtered.length !== selectedMacs.length) {
      selectedMacs = filtered;
    }
  }

  type Badge = { label: string; cls: string; spinner?: boolean };

  function statusBadge(row: (typeof rows)[number]): Badge {
    if (row.installState) {
      switch (row.installState.status) {
        case 'updating':
          return { label: 'updating', cls: 'bg-warning text-dark', spinner: true };
        case 'pending':
          return { label: 'pending', cls: 'bg-secondary' };
        case 'current':
          return { label: 'current', cls: 'bg-success' };
        case 'error':
          return { label: 'error', cls: 'bg-danger' };
        case 'unknown':
          return { label: 'unknown', cls: 'bg-secondary' };
        case 'skipped':
          return { label: 'skipped', cls: 'bg-secondary' };
      }
    }
    if (row.checkErr) return { label: 'error', cls: 'bg-danger' };
    if (hasUpdateOnChannel(row, stage)) return { label: 'update', cls: 'bg-warning text-dark' };
    return { label: 'current', cls: 'bg-success' };
  }

  function formatChecked(): string {
    let latest = 0;
    for (const d of Object.values(devicesByMAC)) {
      if (!d.fw_checked_at) continue;
      const t = Date.parse(d.fw_checked_at);
      if (!isNaN(t) && t > latest) latest = t;
    }
    if (!latest) return '';
    const ago = Math.max(0, Math.floor((Date.now() - latest) / 1000));
    if (ago < 60) return `Checked ${ago}s ago`;
    if (ago < 3600) return `Checked ${Math.floor(ago / 60)}m ago`;
    return `Checked ${Math.floor(ago / 3600)}h ago`;
  }

  $: confirmDevices = rows.filter((r) => selectedMacs.includes(r.mac));
</script>

<section class="page-hero">
  <div class="page-hero-stack">
    <span class="page-kicker">Firmware</span>
    <h1 class="h5 mb-0">Fleet firmware workflow</h1>
    {#if formatChecked()}<span class="text-secondary small">{formatChecked()}</span>{/if}
  </div>
  <div class="page-toolbar">
    <select class="form-select toolbar-select-md" bind:value={stage}>
      <option value="stable">Stable</option>
      <option value="beta">Beta</option>
    </select>
    <button
      class="btn btn-warning text-dark"
      on:click={check}
      disabled={checkStatus.running || installStatus.running}
      >{checkStatus.running ? 'Checking...' : 'Check Firmware'}</button
    >
    <button
      class="btn btn-outline-light"
      on:click={openConfirm}
      disabled={selectedMacs.length === 0 || checkStatus.running || installStatus.running}
      >Update {selectedMacs.length}</button
    >
  </div>
</section>

<ErrorNotice summary={error} details={errorDetails} />

{#if checkStatus.running || checkStatus.total > 0}
  <div class="mb-3">
    <ProgressBar
      done={checkStatus.done}
      total={checkStatus.total}
      running={checkStatus.running}
      label="Check {checkStatus.done}/{checkStatus.total}"
      ariaLabel="Firmware check progress"
    />
  </div>
{/if}

{#if installStatus.running || installStatus.total > 0}
  <div class="mb-3">
    <ProgressBar
      done={installStatus.done}
      total={installStatus.total}
      running={installStatus.running}
      label="Install {installStatus.done}/{installStatus.total}"
      ariaLabel="Firmware install progress"
    />
  </div>
{/if}

<table class="table table-dark table-striped">
  <thead>
    <tr>
      <th></th>
      <th>Name</th>
      <th>Model</th>
      <th>IP</th>
      <th>Current</th>
      <th>Available Stable</th>
      <th>Available Beta</th>
      <th>Status</th>
    </tr>
  </thead>
  <tbody>
    {#each rows as row (row.mac)}
      {@const badge = statusBadge(row)}
      <tr>
        <td>
          <input
            type="checkbox"
            class="form-check-input"
            value={row.mac}
            bind:group={selectedMacs}
            disabled={!hasUpdateOnChannel(row, stage) || installStatus.running}
          />
        </td>
        <td>{row.name || '-'}</td>
        <td>{row.model || '-'}</td>
        <td><a href={`http://${row.ip}/`} target="_blank" rel="noreferrer noopener">{row.ip}</a></td
        >
        <td>{row.currentVer || '-'}</td>
        <td>
          {#if row.stableVer && row.stableUpdate}
            <span class="text-info">{row.stableVer}</span>
          {:else if row.stableVer}
            <span class="text-secondary">{row.stableVer}</span>
          {:else}
            <span class="text-secondary">-</span>
          {/if}
        </td>
        <td>
          {#if row.betaVer && row.betaUpdate}
            <span class="text-info">{row.betaVer}</span>
          {:else if row.betaVer}
            <span class="text-secondary">{row.betaVer}</span>
          {:else}
            <span class="text-secondary">-</span>
          {/if}
        </td>
        <td>
          <span class={`badge ${badge.cls}`}>
            {#if badge.spinner}<span
                class="spinner-border spinner-border-sm me-1"
                role="status"
                aria-hidden="true"
              ></span>{/if}{badge.label}
          </span>
          {#if row.installState?.detail && (row.installState.status === 'error' || row.installState.status === 'unknown')}
            <div class="small text-secondary mt-1">{row.installState.detail}</div>
          {:else if row.checkErr}
            <div class="small text-secondary mt-1">{row.checkErr}</div>
          {/if}
        </td>
      </tr>
    {/each}
    {#if rows.length === 0}
      <tr>
        <td colspan="8" class="text-secondary text-center py-3">No devices known yet.</td>
      </tr>
    {/if}
  </tbody>
</table>

{#if confirmOpen}
  <div
    class="modal-backdrop-custom"
    role="dialog"
    aria-modal="true"
    aria-labelledby="fw-confirm-title"
  >
    <div class="modal-card card text-bg-dark border-secondary">
      <div class="card-body">
        <h2 class="h6 mb-3" id="fw-confirm-title">
          Install {stage.toUpperCase()} firmware on {confirmDevices.length}
          {confirmDevices.length === 1 ? 'device' : 'devices'}?
        </h2>
        <ul class="mb-3 small">
          {#each confirmDevices as d (d.mac)}
            <li>
              <span class="font-monospace">{d.ip}</span>
              <span class="ms-2">{d.name || d.mac}</span>
              {#if d.model}<span class="text-secondary ms-2">{d.model}</span>{/if}
              <span class="ms-2 text-secondary">→</span>
              <span class="ms-2">{stage === 'beta' ? d.betaVer : d.stableVer}</span>
            </li>
          {/each}
        </ul>
        <div class="d-flex justify-content-end gap-2">
          <button class="btn btn-outline-light" on:click={() => (confirmOpen = false)}
            >Cancel</button
          >
          <button class="btn btn-warning text-dark" on:click={confirmAndUpdate}
            >Update {confirmDevices.length}</button
          >
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  .modal-backdrop-custom {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    z-index: 1050;
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding: 4rem 1rem 1rem;
  }
  .modal-card {
    width: min(560px, 100%);
    max-height: calc(100vh - 6rem);
    overflow: auto;
  }
</style>
