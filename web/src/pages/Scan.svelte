<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import { devices, navigate } from '../lib/stores'
  import type { ScanStatus } from '../lib/types'
  import ErrorNotice from '../components/ErrorNotice.svelte'

  let status: ScanStatus = { running: false, found: 0, total: 0, done: 0, pending: [] }
  let timer: number | undefined
  let message = ''
  let error = ''
  let errorDetails = ''

  function ensurePolling() {
    if (!timer) {
      timer = window.setInterval(poll, 2000)
    }
  }

  async function poll() {
    try {
      status = await api.scanStatus()
      if (status.running) {
        message = `Scan running: ${status.done} of ${status.total || '?'} targets checked.`
        ensurePolling()
      } else if (status.total > 0) {
        message = `Last scan finished: ${status.found} device${status.found === 1 ? '' : 's'} found.`
      }
      if (!status.running && timer) {
        clearInterval(timer)
        timer = undefined
      }
    } catch (err) {
      error = (err as Error).message
      errorDetails = String(err)
    }
  }

  async function start() {
    error = ''
    errorDetails = ''
    message = ''
    try {
      await api.scanStart()
      message = 'Scan started. Checking for devices now...'
      await poll()
      ensurePolling()
    } catch (err) {
      error = (err as Error).message
      errorDetails = String(err)
    }
  }

  async function addAll() {
    await confirmAndRefresh()
  }

  async function addNewOnly() {
    await confirmAndRefresh(status.pending.filter((d) => d.is_new).map((d) => d.mac))
  }

  async function confirmAndRefresh(macs?: string[]) {
    error = ''
    errorDetails = ''
    message = ''
    try {
      const result = await api.scanConfirm(macs)
      $devices = await api.getDevices()
      await poll()
      message = `Added ${result.count} device${result.count === 1 ? '' : 's'} to inventory.`
      if (result.count > 0) {
        navigate('/')
      }
    } catch (err) {
      error = (err as Error).message
      errorDetails = String(err)
    }
  }

  onMount(() => {
    void poll()
    return () => {
      if (timer) {
        clearInterval(timer)
      }
    }
  })
</script>

<section class="page-hero">
  <div class="page-hero-stack">
    <span class="page-kicker">Scan</span>
    <h1 class="h5 mb-0">Discovery workflow</h1>
  </div>
  <div class="page-toolbar">
    <button class="btn btn-warning text-dark" on:click={start} disabled={status.running}>Start Scan</button>
    <button class="btn btn-outline-light" on:click={poll}>Refresh Status</button>
    <button class="btn btn-outline-success" on:click={addAll} disabled={status.pending.length === 0}>Add All</button>
    <button class="btn btn-outline-warning" on:click={addNewOnly} disabled={!status.pending.some((d) => d.is_new)}>Add New Only</button>
  </div>
</section>

{#if message}
  <div class="alert alert-success" role="status" aria-live="polite">{message}</div>
{/if}

<ErrorNotice summary={error} details={errorDetails} />

<div class="card bg-dark border-secondary mb-3" role="status" aria-live="polite" aria-busy={status.running}>
  <div class="card-body">
    <div class="d-flex justify-content-between align-items-center flex-wrap gap-2">
      <div>
        <div class="fw-bold">{status.running ? 'Scan in progress' : 'Scan idle'}</div>
        <div class="text-secondary">{status.found} device{status.found === 1 ? '' : 's'} found, {status.pending.filter((d) => d.is_new).length} new</div>
      </div>
      <div class="text-secondary">{status.done} / {status.total} targets checked</div>
    </div>
  </div>
</div>

<div class="progress mb-3" role="progressbar" aria-valuenow={status.done} aria-valuemin="0" aria-valuemax={status.total || 100} aria-label="Scan progress">
  <div class="progress-bar progress-bar-striped" style={`width:${status.total ? (status.done / status.total) * 100 : 0}%`}>
    {status.done} / {status.total}
  </div>
</div>

<div class="row g-3">
  <div class="col-lg-6">
    <div class="card bg-dark border-warning">
      <div class="card-header">New Devices</div>
      <div class="list-group list-group-flush">
        {#each status.pending.filter((d) => d.is_new) as device}
          <div class="list-group-item list-group-item-dark">{device.ip} · {device.name || device.mac}</div>
        {/each}
      </div>
    </div>
  </div>
  <div class="col-lg-6">
    <div class="card bg-dark border-secondary">
      <div class="card-header">Known Devices</div>
      <div class="list-group list-group-flush">
        {#each status.pending.filter((d) => !d.is_new) as device}
          <div class="list-group-item list-group-item-dark">{device.ip} · {device.name || device.mac}</div>
        {/each}
      </div>
    </div>
  </div>
</div>
