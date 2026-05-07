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

  // Actions that produce unrecoverable device state (no rollback path from
  // the app side) require the operator to type the device name before they
  // fire — see ADR-0002 carve-out documented in
  // docs/plans/broader-action-discovery.md. Reversible high-risk actions
  // like firmware_update keep the existing single-click behaviour.
  const TYPED_CONFIRM_ACTIONS = new Set(['factory_reset', 'factory_reset_wifi', 'ota_revert']);

  let confirmAction: { id: string; label: string; description: string } | null = null;
  let confirmTyped = '';
  $: confirmExpected = detail?.device.name || detail?.device.mac || '';
  $: confirmReady = confirmTyped.trim() === confirmExpected;

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
    // High-risk + unrecoverable actions get a typed-name gate before
    // any RPC fires. Other actions go through immediately.
    if (TYPED_CONFIRM_ACTIONS.has(action)) {
      const def = detail?.actions.find((a) => a.id === action);
      if (def) {
        confirmAction = { id: def.id, label: def.label, description: def.description };
        confirmTyped = '';
        return;
      }
    }
    await runActionImmediate(action);
  }

  async function runActionImmediate(action: string) {
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

  async function confirmAndRun() {
    if (!confirmAction || !confirmReady) return;
    const id = confirmAction.id;
    confirmAction = null;
    confirmTyped = '';
    await runActionImmediate(id);
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
      {#if detail?.device.app}
        <span
          class="badge bg-secondary align-middle ms-2"
          style="font-size: 0.55em; vertical-align: middle;"
          title={detail.device.model
            ? `App: ${detail.device.app}\nModel: ${detail.device.model}`
            : detail.device.app}
        >
          {detail.device.app}
        </span>
      {/if}
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
              <span class="text-secondary">Type</span><strong
                title={detail.device.app && detail.device.model
                  ? `App: ${detail.device.app}\nModel: ${detail.device.model}`
                  : ''}>{detail.device.app || detail.device.model || 'n/a'}</strong
              >
            </div>
            <div>
              <span class="text-secondary">Model SKU</span><strong
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
            {#if detail.device.switch_count || detail.device.cover_count || detail.device.light_count}
              <div>
                <span class="text-secondary">Components</span><strong>
                  {[
                    detail.device.switch_count
                      ? `${detail.device.switch_count} switch${detail.device.switch_count === 1 ? '' : 'es'}`
                      : '',
                    detail.device.cover_count
                      ? `${detail.device.cover_count} cover${detail.device.cover_count === 1 ? '' : 's'}`
                      : '',
                    detail.device.light_count
                      ? `${detail.device.light_count} light${detail.device.light_count === 1 ? '' : 's'}`
                      : '',
                  ]
                    .filter(Boolean)
                    .join(', ')}
                </strong>
              </div>
            {/if}
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
            <div>
              <span class="text-secondary">Scheme</span><strong class="font-monospace"
                >{detail.device.scheme || 'http'}</strong
              >
            </div>
            {#if detail.device.enhanced_security !== null && detail.device.enhanced_security !== undefined}
              <div>
                <span class="text-secondary">Enhanced Security</span><strong
                  >{detail.device.enhanced_security ? 'on' : 'off'}</strong
                >
              </div>
            {/if}
            {#if detail.device.tls_cert_valid !== null && detail.device.tls_cert_valid !== undefined}
              <div>
                <span class="text-secondary">TLS Cert</span><strong
                  >{detail.device.tls_cert_valid ? 'valid' : 'invalid'}</strong
                >
              </div>
            {/if}
            {#if detail.device.auth_locked_until}
              <div>
                <span class="text-secondary">Auth Locked Until</span><strong
                  title={formatDateTime(detail.device.auth_locked_until)}
                  >{formatRelativeDateTime(detail.device.auth_locked_until)}</strong
                >
              </div>
            {/if}
            {#if detail.device.wifi_hostname}
              <div>
                <span class="text-secondary">Hostname</span><strong class="font-monospace"
                  >{detail.device.wifi_hostname}</strong
                >
              </div>
            {/if}
            {#if detail.device.wifi_channel}
              <div>
                <span class="text-secondary">WiFi Channel</span><strong
                  >{detail.device.wifi_channel}</strong
                >
              </div>
            {/if}
          </div>
        </div>
      </div>

      {#if detail.device.power_w !== null && detail.device.power_w !== undefined}
        <div class="card bg-dark border-secondary mb-3">
          <div class="card-body">
            <h2 class="h5">Live Readings</h2>
            <div class="detail-grid">
              <div>
                <span class="text-secondary">Power</span><strong class="font-monospace"
                  >{detail.device.power_w?.toFixed(1)} W</strong
                >
              </div>
              {#if detail.device.voltage_v !== null && detail.device.voltage_v !== undefined}
                <div>
                  <span class="text-secondary">Voltage</span><strong class="font-monospace"
                    >{detail.device.voltage_v?.toFixed(1)} V</strong
                  >
                </div>
              {/if}
              {#if detail.device.current_a !== null && detail.device.current_a !== undefined}
                <div>
                  <span class="text-secondary">Current</span><strong class="font-monospace"
                    >{detail.device.current_a?.toFixed(2)} A</strong
                  >
                </div>
              {/if}
            </div>
            <div class="text-secondary mt-2" style="font-size: 0.78rem;">
              Summed across switch / em / em1 / pm1 components from the most recent snapshot.
            </div>
          </div>
        </div>
      {/if}

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

{#if confirmAction}
  <div
    class="modal-backdrop-custom"
    role="dialog"
    aria-modal="true"
    aria-labelledby="action-confirm-title"
  >
    <div class="modal-card card text-bg-dark border-warning">
      <div class="card-body">
        <h2 class="h6 mb-3" id="action-confirm-title">
          <span class="badge bg-warning text-dark me-2">Unrecoverable</span>
          {confirmAction.label}
        </h2>
        <p class="mb-2">{confirmAction.description}</p>
        <p class="text-secondary small mb-3">
          Type the device's name <code>{confirmExpected}</code> to confirm.
        </p>
        <input
          class="form-control mb-3"
          type="text"
          placeholder={confirmExpected}
          bind:value={confirmTyped}
          autocomplete="off"
        />
        <div class="d-flex justify-content-end gap-2">
          <button
            class="btn btn-outline-light"
            on:click={() => {
              confirmAction = null;
              confirmTyped = '';
            }}>Cancel</button
          >
          <button
            class="btn btn-danger"
            on:click={confirmAndRun}
            disabled={!confirmReady}
            title={confirmReady
              ? 'Run the destructive action'
              : 'Type the device name exactly to enable'}>{confirmAction.label}</button
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
