<script lang="ts">
  import { onDestroy } from 'svelte';
  import { APIError, api } from '../lib/api';
  import { firmwareChannel, type FirmwareChannel } from '../lib/stores';
  import type {
    AppSettings,
    Device,
    FWResult,
    FirmwareInstallResult,
    FirmwareInstallStatus,
    FirmwareStatus,
  } from '../lib/types';
  import { genBadgeClass, genLabel, genTitle } from '../lib/genBadge';
  import ErrorNotice from '../components/ErrorNotice.svelte';
  import ProgressBar from '../components/ProgressBar.svelte';
  import SortHeader from '../components/SortHeader.svelte';

  type Channel = FirmwareChannel;

  // Bound to a global persisted store (shared with Devices page).
  let stage: Channel;
  firmwareChannel.subscribe((v) => (stage = v));
  function setStage(v: Channel) {
    firmwareChannel.set(v);
  }
  let checkStatus: FirmwareStatus = { running: false, done: 0, total: 0, results: [] };
  let installStatus: FirmwareInstallStatus = { running: false, done: 0, total: 0, results: [] };
  let devicesByMAC: Record<string, Device> = {};
  let appSettings: AppSettings | null = null;
  let selectedMacs: string[] = [];
  let checkTimer: number | undefined;
  let installTimer: number | undefined;
  let installOverlayActive = false; // hide stale install state once a fresh check completes
  let sortKey = 'name';
  let sortDir: 'asc' | 'desc' = 'asc';
  let confirmOpen = false;

  function setSort(key: string) {
    if (sortKey === key) {
      sortDir = sortDir === 'asc' ? 'desc' : 'asc';
    } else {
      sortKey = key;
      sortDir = 'asc';
    }
  }
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
    if (s.running) {
      installOverlayActive = true;
      startInstallPolling();
    } else {
      installOverlayActive = false;
    }
  });
  api
    .getSettings()
    .then((s) => (appSettings = s))
    .catch(() => undefined);

  function startCheckPolling() {
    if (checkTimer) return;
    checkTimer = window.setInterval(async () => {
      try {
        checkStatus = await api.firmwareStatus();
        if (!checkStatus.running) {
          stopCheckPolling();
          // Re-pull devices so Available columns reflect freshly persisted cache.
          refreshDeviceIndex();
          // The user just took a fresh check — drop any stale install
          // overlay so each row reflects current reality.
          installOverlayActive = false;
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
    if (updateEligibleMacs.length === 0) return;
    try {
      await api.firmwareUpdate(updateEligibleMacs, stage);
      installStatus = { ...installStatus, running: true };
      installOverlayActive = true;
      startInstallPolling();
    } catch (err) {
      captureError(err);
    }
  }

  let autoUpdateBusy = false;
  let autoUpdateStatus = '';

  async function applyAutoUpdate(mode: 'off' | 'stable' | 'beta') {
    if (selectedMacs.length === 0) return;
    error = '';
    errorDetails = '';
    autoUpdateStatus = '';
    autoUpdateBusy = true;
    try {
      const res = await api.bulk({
        action: 'set_auto_update',
        macs: [...selectedMacs],
        value: mode,
      });
      const ok = res.results.filter((r) => r.status === 'ok').length;
      const failed = res.results.length - ok;
      autoUpdateStatus = `Auto-update → ${mode}: ${ok} ok${failed ? `, ${failed} failed` : ''}`;
      await refreshDeviceIndex();
    } catch (err) {
      captureError(err);
    } finally {
      autoUpdateBusy = false;
    }
  }

  function openConfirm() {
    if (updateEligibleMacs.length === 0) return;
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
    if (!installOverlayActive) return m;
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
        app: d.app,
        gen: d.gen,
        switchCount: d.switch_count,
        coverCount: d.cover_count,
        lightCount: d.light_count,
        fwAlt: d.fw_alt ?? [],
        fwFrozen: d.fw_frozen ?? false,
        currentVer: d.fw,
        stableVer,
        betaVer,
        stableUpdate: stableVer !== '' && stableVer !== d.fw,
        betaUpdate: betaVer !== '' && betaVer !== d.fw,
        autoUpdate: d.fw_auto_update,
        installState: i,
        checkErr,
      };
    });

  // Sort comparator for the table. Falls back to device_num when the chosen
  // column has identical values so adjacent ties stay in insertion order.
  function sortValueFor(row: (typeof rows)[number], key: string): string | number {
    switch (key) {
      case 'name':
        return (row.name || '').toLowerCase();
      case 'gen':
        return row.gen;
      case 'model':
        return (row.app || row.model || '').toLowerCase();
      case 'ip': {
        // Numeric octet sort so 192.168.211.9 < 192.168.211.10.
        const parts = (row.ip || '').split('.');
        return parts.reduce((acc, oct, i) => acc + Number(oct || 0) * Math.pow(256, 3 - i), 0);
      }
      case 'current':
        return (row.currentVer || '').toLowerCase();
      case 'stable':
        return (row.stableVer || '').toLowerCase();
      case 'beta':
        return (row.betaVer || '').toLowerCase();
      case 'auto_update':
        return (row.autoUpdate || '').toLowerCase();
      case 'status':
        // Group by derived status label for predictable ordering.
        return statusBadge(row).label;
      default:
        return '';
    }
  }

  $: sortedRows = (() => {
    const copy = [...rows];
    copy.sort((a, b) => {
      const av = sortValueFor(a, sortKey);
      const bv = sortValueFor(b, sortKey);
      let cmp: number;
      if (typeof av === 'number' && typeof bv === 'number') cmp = av - bv;
      else cmp = String(av).localeCompare(String(bv));
      return sortDir === 'asc' ? cmp : -cmp;
    });
    return copy;
  })();

  function autoUpdateBadge(mode: string): { label: string; cls: string } {
    switch (mode) {
      case 'stable':
        return { label: 'stable', cls: 'bg-success' };
      case 'beta':
        return { label: 'beta', cls: 'bg-info text-dark' };
      case 'off':
        return { label: 'off', cls: 'bg-secondary' };
      default:
        return { label: 'unknown', cls: 'bg-dark border border-secondary' };
    }
  }

  function hasUpdateOnChannel(row: { stableUpdate: boolean; betaUpdate: boolean }, ch: Channel) {
    return ch === 'beta' ? row.betaUpdate : row.stableUpdate;
  }

  // (Was: auto-uncheck rows that don't have an update on the active channel.
  // Removed in favour of channel-agnostic selection so the auto-update bulk
  // buttons can target current-firmware devices. The Update flow now filters
  // to `updateEligibleMacs` at the action level.)

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

  $: confirmDevices = rows.filter((r) => updateEligibleMacs.includes(r.mac));

  // Select-all spans every row so the auto-update bulk action can target
  // devices that are already on the latest firmware. The Update flow filters
  // internally to rows with an available update on the active channel — see
  // `updateEligibleCount` below.
  $: selectableMacs = rows.map((r) => r.mac);
  $: allChecked =
    selectableMacs.length > 0 && selectableMacs.every((m) => selectedMacs.includes(m));
  $: someChecked = selectableMacs.some((m) => selectedMacs.includes(m));
  $: updateEligibleMacs = selectedMacs.filter((m) => {
    const r = rows.find((x) => x.mac === m);
    return r ? hasUpdateOnChannel(r, stage) : false;
  });

  function toggleAll(e: Event) {
    const target = e.currentTarget as HTMLInputElement;
    if (target.checked) {
      // Local dedup helper; results spread into selectedMacs (a plain array).
      // eslint-disable-next-line svelte/prefer-svelte-reactivity
      const set = new Set(selectedMacs);
      for (const m of selectableMacs) set.add(m);
      selectedMacs = [...set];
    } else {
      const drop = new Set(selectableMacs);
      selectedMacs = selectedMacs.filter((m) => !drop.has(m));
    }
  }
</script>

<section class="page-hero">
  <div class="page-hero-stack">
    <span class="page-kicker">Firmware</span>
    <h1 class="h5 mb-0">Fleet firmware workflow</h1>
    {#if formatChecked()}<span class="text-secondary small">{formatChecked()}</span>{/if}
  </div>
  <div class="page-toolbar">
    <select
      class="form-select toolbar-select-md"
      value={stage}
      on:change={(e) => setStage((e.currentTarget as HTMLSelectElement).value as Channel)}
    >
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
      disabled={updateEligibleMacs.length === 0 || checkStatus.running || installStatus.running}
      title={updateEligibleMacs.length === selectedMacs.length
        ? `Update ${updateEligibleMacs.length} selected device(s)`
        : `Update ${updateEligibleMacs.length} of ${selectedMacs.length} selected (rest have no update on the ${stage} channel)`}
      >Update {updateEligibleMacs.length}{selectedMacs.length !== updateEligibleMacs.length
        ? `/${selectedMacs.length}`
        : ''}</button
    >
    <div
      class="btn-group"
      role="group"
      aria-label="Auto-update bulk action for the selected devices"
    >
      <button
        type="button"
        class="btn btn-outline-secondary"
        on:click={() => applyAutoUpdate('off')}
        disabled={selectedMacs.length === 0 || autoUpdateBusy || installStatus.running}
        title="Set selected devices' local auto-update schedule to OFF"
      >
        Auto → Off
      </button>
      <button
        type="button"
        class="btn btn-outline-success"
        on:click={() => applyAutoUpdate('stable')}
        disabled={selectedMacs.length === 0 || autoUpdateBusy || installStatus.running}
        title="Set selected devices to auto-install Stable firmware nightly"
      >
        Auto → Stable
      </button>
      <button
        type="button"
        class="btn btn-outline-info"
        on:click={() => applyAutoUpdate('beta')}
        disabled={selectedMacs.length === 0 || autoUpdateBusy || installStatus.running}
        title="Set selected devices to auto-install Beta firmware nightly"
      >
        Auto → Beta
      </button>
    </div>
  </div>
</section>

<ErrorNotice summary={error} details={errorDetails} />

{#if autoUpdateStatus}
  <div class="alert alert-secondary py-2 mb-3 d-flex align-items-center gap-2" role="status">
    <span class="flex-grow-1">{autoUpdateStatus}</span>
    <button
      type="button"
      class="btn-close btn-close-white"
      aria-label="Dismiss"
      on:click={() => (autoUpdateStatus = '')}
    ></button>
  </div>
{/if}

{#if checkStatus.running}
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

{#if installStatus.running}
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

<div class="table-responsive">
  <table class="table table-dark table-striped">
    <thead>
      <tr>
        <th>
          <input
            type="checkbox"
            class="form-check-input"
            aria-label="Select every device in the table"
            title={selectableMacs.length === 0
              ? 'No devices to select'
              : `Select all ${selectableMacs.length} devices`}
            checked={allChecked}
            indeterminate={!allChecked && someChecked}
            disabled={selectableMacs.length === 0 || installStatus.running}
            on:change={toggleAll}
          />
        </th>
        <SortHeader label="Name" column="name" {sortKey} {sortDir} onSort={setSort} />
        <SortHeader label="Gen" column="gen" {sortKey} {sortDir} onSort={setSort} />
        <SortHeader label="Model" column="model" {sortKey} {sortDir} onSort={setSort} />
        <SortHeader label="IP" column="ip" {sortKey} {sortDir} onSort={setSort} />
        <SortHeader label="Current" column="current" {sortKey} {sortDir} onSort={setSort} />
        <SortHeader label="Available Stable" column="stable" {sortKey} {sortDir} onSort={setSort} />
        <SortHeader label="Available Beta" column="beta" {sortKey} {sortDir} onSort={setSort} />
        <SortHeader label="Auto-Update" column="auto_update" {sortKey} {sortDir} onSort={setSort} />
        <SortHeader label="Status" column="status" {sortKey} {sortDir} onSort={setSort} />
      </tr>
    </thead>
    <tbody>
      {#each sortedRows as row (row.mac)}
        {@const badge = statusBadge(row)}
        {@const ab = autoUpdateBadge(row.autoUpdate)}
        <tr>
          <td>
            <input
              type="checkbox"
              class="form-check-input"
              value={row.mac}
              bind:group={selectedMacs}
              disabled={installStatus.running}
              title={hasUpdateOnChannel(row, stage)
                ? 'Eligible for both update and auto-update bulk actions'
                : 'Eligible for auto-update bulk actions only (no update on this channel)'}
            />
          </td>
          <td>{row.name || '-'}</td>
          <td>
            <span class={`badge ${genBadgeClass(row.gen, appSettings)}`} title={genTitle(row.gen)}
              >{genLabel(row.gen)}</span
            >
          </td>
          <td>
            {#if row.app || row.model}
              {@const tip = [
                row.app ? `App: ${row.app}` : '',
                row.model ? `Model: ${row.model}` : '',
                `Gen ${row.gen}`,
                row.switchCount ? `Switch: ${row.switchCount}` : '',
                row.coverCount ? `Cover: ${row.coverCount}` : '',
                row.lightCount ? `Light: ${row.lightCount}` : '',
              ]
                .filter(Boolean)
                .join('\n')}
              <div title={tip}>
                {row.app || row.model}
              </div>
            {:else}
              -
            {/if}
            {#each row.fwAlt as alt (alt.id)}
              <span
                class="badge bg-warning text-dark ms-1"
                title={`Alternative firmware available: ${alt.desc || alt.name}${alt.stable ? `\nstable: ${alt.stable}` : ''}${alt.beta ? `\nbeta: ${alt.beta}` : ''}\n(read-only — cannot be switched via ShellyAdmin)`}
              >
                alt: {alt.id}
              </span>
            {/each}
            {#if row.fwFrozen}
              <span
                class="badge bg-secondary ms-1"
                title="This firmware line is feature-frozen at its current stable version and will never receive 2.0.0+ (Shelly Firmware Update Policy). Informational only — does not block updates/installs."
              >
                frozen
              </span>
            {/if}
          </td>
          <td
            ><a href={`http://${row.ip}/`} target="_blank" rel="noreferrer noopener">{row.ip}</a
            ></td
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
            <span class={`badge ${ab.cls}`}>{ab.label}</span>
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
      {#if sortedRows.length === 0}
        <tr>
          <td colspan="10" class="text-secondary text-center py-3">No devices known yet.</td>
        </tr>
      {/if}
    </tbody>
  </table>
</div>

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
