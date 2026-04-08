<script lang="ts">
  import { api } from '../lib/api'
  import { devices, navigate } from '../lib/stores'
  import type { ScanStatus } from '../lib/types'

  let status: ScanStatus = { running: false, found: 0, total: 0, done: 0, pending: [] }
  let timer: number | undefined
  let message = ''
  let error = ''

  async function poll() {
    status = await api.scanStatus()
    if (!status.running && timer) {
      clearInterval(timer)
      timer = undefined
    }
  }

  async function start() {
    error = ''
    message = ''
    await api.scanStart()
    await poll()
    timer = window.setInterval(poll, 2000)
  }

  async function addAll() {
    await confirmAndRefresh()
  }

  async function addNewOnly() {
    await confirmAndRefresh(status.pending.filter((d) => d.is_new).map((d) => d.mac))
  }

  async function confirmAndRefresh(macs?: string[]) {
    error = ''
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
    }
  }
</script>

<div class="d-flex gap-2 mb-3">
  <button class="btn btn-warning text-dark" on:click={start} disabled={status.running}>Start Scan</button>
  <button class="btn btn-outline-light" on:click={poll}>Refresh Status</button>
  <button class="btn btn-outline-success" on:click={addAll} disabled={status.pending.length === 0}>Add All</button>
  <button class="btn btn-outline-warning" on:click={addNewOnly} disabled={!status.pending.some((d) => d.is_new)}>Add New Only</button>
</div>

{#if message}
  <div class="alert alert-success">{message}</div>
{/if}

{#if error}
  <div class="alert alert-danger">{error}</div>
{/if}

<div class="progress mb-3" role="progressbar">
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
