<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import type { LogEntry } from '../lib/types'

  let level = ''
  let search = ''
  let logs: LogEntry[] = []
  let busy = false
  let notice = ''
  let error = ''

  async function load() {
    error = ''
    logs = await api.getLogs(level, search)
  }

  async function clearAll() {
    const confirmed = window.confirm('Delete all logs? This cannot be undone.')
    if (!confirmed) return
    busy = true
    error = ''
    notice = ''
    try {
      const result = await api.clearLogs()
      notice = `Deleted ${result.deleted} log entries.`
      await load()
    } catch (err) {
      error = (err as Error).message
    } finally {
      busy = false
    }
  }

  onMount(() => void load())
</script>

<div class="d-flex gap-2 mb-3">
  <select class="form-select toolbar-select-md" bind:value={level}>
    <option value="">All Levels</option>
    <option value="INFO">INFO</option>
    <option value="WARN">WARN</option>
    <option value="ERROR">ERROR</option>
  </select>
  <input class="form-control" placeholder="Search logs" bind:value={search} />
  <button class="btn btn-warning text-dark" on:click={load}>Load</button>
  <button class="btn btn-outline-danger" on:click={clearAll} disabled={busy}>Delete All Logs</button>
</div>

{#if notice}
  <div class="alert alert-success py-2">{notice}</div>
{/if}
{#if error}
  <div class="alert alert-danger py-2">{error}</div>
{/if}

<table class="table table-dark table-striped">
  <thead><tr><th>Timestamp</th><th>Level</th><th>Message</th></tr></thead>
  <tbody>
    {#each logs as log}
      <tr>
        <td>{log.ts}</td>
        <td><span class={`badge ${log.level === 'ERROR' ? 'bg-danger' : log.level === 'WARN' ? 'bg-warning text-dark' : log.level === 'DEBUG' ? 'bg-info text-dark' : 'bg-secondary'}`}>{log.level}</span></td>
        <td>{log.message}</td>
      </tr>
    {/each}
  </tbody>
</table>
