<script lang="ts">
  import { onMount } from 'svelte';
  import { APIError, api, triggerDownload } from '../lib/api';
  import { currentPath, navigate } from '../lib/stores';
  import { formatDateTime, formatRelativeDateTime } from '../lib/time';
  import type { DeviceActionResult, DeviceDetail } from '../lib/types';
  import ErrorNotice from '../components/ErrorNotice.svelte';

  let detail: DeviceDetail | null = null;
  let error = '';
  let errorDetails = '';
  let actionMessage = '';
  let actionResult: DeviceActionResult | null = null;
  let busyAction = '';
  let selectedStage = 'stable';

  $: target = decodeURIComponent($currentPath.replace('/devices/', ''));

  function captureError(err: unknown) {
    if (err instanceof APIError) {
      error = err.message;
      errorDetails = `${err.method} ${err.path} -> ${err.status}\n${JSON.stringify(err.detail ?? {}, null, 2)}`;
      return;
    }
    error = (err as Error).message;
    errorDetails = String(err);
  }

  async function load() {
    error = '';
    errorDetails = '';
    actionMessage = '';
    try {
      detail = await api.getDeviceDetail(target);
    } catch (err) {
      captureError(err);
    }
  }

  async function runAction(action: string) {
    busyAction = action;
    actionMessage = '';
    actionResult = null;
    error = '';
    errorDetails = '';
    try {
      const payload = action.startsWith('firmware_') ? { stage: selectedStage } : {};
      actionResult = await api.runDeviceAction(target, action, payload);
      actionMessage = actionResult.detail;
      await load();
    } catch (err) {
      captureError(err);
    } finally {
      busyAction = '';
    }
  }

  function pretty(value: unknown): string {
    return JSON.stringify(value, null, 2);
  }

  async function exportDevice() {
    try {
      const payload = await api.exportDevice(target);
      const identifier = (payload.device.mac || payload.device.ip || 'device').replace(/:/g, '');
      const stamp = new Date().toISOString().replace(/[-:]/g, '').split('.')[0] + 'Z';
      const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
      triggerDownload(`shellyadmin-device-${identifier}-${stamp}.json`, blob);
    } catch (err) {
      captureError(err);
    }
  }

  onMount(() => {
    void load();
  });
</script>

<section class="page-hero">
  <div class="page-hero-stack">
    <button class="btn btn-outline-light btn-sm mb-2" on:click={() => navigate('/')}
      >Back to Devices</button
    >
    <h1 class="h4 mb-0">
      {detail?.device.name || detail?.device.serial || detail?.device.mac || target}
    </h1>
    <p class="text-secondary mb-0">{detail?.device.ip || target}</p>
  </div>
  <div class="page-toolbar">
    <label class="form-label mb-0" for="device-stage">Firmware channel</label>
    <select id="device-stage" class="form-select toolbar-select-md" bind:value={selectedStage}>
      <option value="stable">Stable</option>
      <option value="beta">Beta</option>
    </select>
    <button class="btn btn-sm btn-outline-light" on:click={exportDevice} disabled={!detail}
      >Export JSON</button
    >
  </div>
</section>

<ErrorNotice summary={error} details={errorDetails} />

{#if actionMessage}
  <div class="alert alert-secondary mb-3">{actionMessage}</div>
{/if}

{#if detail}
  <div class="row g-3">
    <div class="col-lg-5">
      <div class="card bg-dark border-secondary mb-3">
        <div class="card-body">
          <h2 class="h5">Status</h2>
          <div class="detail-grid">
            <div>
              <span class="text-secondary">Model</span><strong
                >{detail.device.model || 'n/a'}</strong
              >
            </div>
            <div>
              <span class="text-secondary">Firmware</span><strong
                >{detail.device.fw || 'n/a'}</strong
              >
            </div>
            <div>
              <span class="text-secondary">Generation</span><strong>Gen {detail.device.gen}</strong>
            </div>
            <div>
              <span class="text-secondary">Online</span><strong
                >{detail.device.online ? 'Yes' : 'No'}</strong
              >
            </div>
            <div>
              <span class="text-secondary">Last Success</span>
              <strong
                title={detail.device.last_seen ? formatDateTime(detail.device.last_seen) : 'never'}
              >
                {detail.device.last_seen
                  ? formatRelativeDateTime(detail.device.last_seen)
                  : 'never'}
              </strong>
            </div>
            <div>
              <span class="text-secondary">Auth</span><strong
                >{detail.device.auth_required
                  ? detail.device.auth_error || 'required'
                  : 'clear'}</strong
              >
            </div>
          </div>
        </div>
      </div>

      <div class="card bg-dark border-secondary mb-3">
        <div class="card-body">
          <h2 class="h5">Capabilities</h2>
          <div class="d-flex flex-column gap-2">
            {#each detail.capabilities as capability}
              <div class="capability-row">
                <span>{capability.label}</span>
                <span class="badge bg-secondary">{capability.state}</span>
              </div>
            {/each}
          </div>
        </div>
      </div>

      <div class="card bg-dark border-secondary">
        <div class="card-body">
          <h2 class="h5">Actions</h2>
          <div class="d-flex flex-column gap-2">
            {#each detail.actions as action}
              <div class="action-row">
                <div>
                  <div class="fw-bold">{action.label}</div>
                  <div class="text-secondary">{action.description}</div>
                  {#if !action.supported && action.reason}
                    <div class="text-secondary">Unavailable: {action.reason}</div>
                  {/if}
                </div>
                <button
                  class="btn btn-sm {action.risk === 'high'
                    ? 'btn-warning text-dark'
                    : 'btn-outline-light'}"
                  disabled={!action.supported || busyAction === action.id}
                  on:click={() => runAction(action.id)}
                >
                  {busyAction === action.id ? 'Running…' : action.label}
                </button>
              </div>
            {/each}
          </div>
        </div>
      </div>
    </div>

    <div class="col-lg-7">
      <div class="card bg-dark border-secondary mb-3">
        <div class="card-body">
          <h2 class="h5">Compliance</h2>
          {#if detail.device.compliance_issues?.length}
            <ul class="mb-0">
              {#each detail.device.compliance_issues as issue}
                <li>{issue}</li>
              {/each}
            </ul>
          {:else}
            <div class="alert alert-secondary mb-0">
              No compliance issues detected in the latest snapshot.
            </div>
          {/if}
        </div>
      </div>

      <div class="card bg-dark border-secondary mb-3">
        <div class="card-body">
          <h2 class="h5">Raw Config</h2>
          <pre class="mb-0 raw-block">{pretty(detail.raw_config)}</pre>
        </div>
      </div>

      <div class="card bg-dark border-secondary">
        <div class="card-body">
          <h2 class="h5">Raw Status</h2>
          <pre class="mb-0 raw-block">{pretty(detail.raw_status)}</pre>
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  .detail-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.85rem;
  }

  .detail-grid div {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .capability-row,
  .action-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1rem;
    padding: 0.75rem;
    border: 1px solid rgba(255, 255, 255, 0.06);
    border-radius: 0.6rem;
    background: rgba(255, 255, 255, 0.02);
  }

  .raw-block {
    max-height: 28rem;
    overflow: auto;
    padding: 0.75rem;
    border-radius: 0.6rem;
    background: rgba(0, 0, 0, 0.25);
  }

  @media (max-width: 900px) {
    .detail-grid {
      grid-template-columns: 1fr;
    }

    .capability-row,
    .action-row {
      flex-direction: column;
      align-items: stretch;
    }
  }
</style>
