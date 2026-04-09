<script lang="ts">
  import { APIError, api } from '../lib/api'
  import type { FirmwareStatus } from '../lib/types'
  import ErrorNotice from '../components/ErrorNotice.svelte'

  let stage = 'stable'
  let status: FirmwareStatus = { running: false, done: 0, total: 0, results: [] }
  let selected = new Set<string>()
  let timer: number | undefined
  let error = ''
  let errorDetails = ''

  function captureError(err: unknown) {
    if (err instanceof APIError) {
      error = err.message
      errorDetails = `${err.method} ${err.path} -> ${err.status}\n${JSON.stringify(err.detail ?? {}, null, 2)}`
      return
    }
    error = (err as Error).message
    errorDetails = String(err)
  }

  async function start() {
    error = ''
    errorDetails = ''
    try {
      await api.firmwareCheck(stage)
      timer = window.setInterval(async () => {
        status = await api.firmwareStatus()
        if (!status.running && timer) {
          clearInterval(timer)
          timer = undefined
        }
      }, 2000)
    } catch (err) {
      captureError(err)
    }
  }

  async function updateSelected() {
    error = ''
    errorDetails = ''
    try {
      await api.firmwareUpdate([...selected], stage)
    } catch (err) {
      captureError(err)
    }
  }
</script>

<div class="d-flex gap-2 mb-3">
  <select class="form-select" bind:value={stage} style="width: 12rem">
    <option value="stable">Stable</option>
    <option value="beta">Beta</option>
  </select>
  <button class="btn btn-warning text-dark" on:click={start}>Check Firmware</button>
  <button class="btn btn-outline-light" on:click={updateSelected} disabled={selected.size === 0}>Update {selected.size}</button>
</div>

<ErrorNotice summary={error} details={errorDetails} />

<div class="progress mb-3"><div class="progress-bar" style={`width:${status.total ? (status.done / status.total) * 100 : 0}%`}>{status.done}/{status.total}</div></div>

<table class="table table-dark table-striped">
  <thead><tr><th></th><th>IP</th><th>Current</th><th>Available</th><th>Status</th></tr></thead>
  <tbody>
    {#each status.results as result}
      <tr>
        <td><input type="checkbox" class="form-check-input" disabled={!result.update_available} on:change={(e) => (e.currentTarget as HTMLInputElement).checked ? selected.add(result.mac) : selected.delete(result.mac)} /></td>
        <td>{result.ip}</td>
        <td>{result.current_ver}</td>
        <td>{result.available_ver || 'n/a'}</td>
        <td><span class={`badge ${result.status === 'update' ? 'bg-warning text-dark' : result.status === 'current' ? 'bg-success' : 'bg-secondary'}`}>{result.status}</span></td>
      </tr>
    {/each}
  </tbody>
</table>
