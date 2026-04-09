<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import type { LogEntry } from '../lib/types'

  let level = ''
  let search = ''
  let logs: LogEntry[] = []

  async function load() {
    logs = await api.getLogs(level, search)
  }

  onMount(() => void load())
</script>

<div class="d-flex gap-2 mb-3">
  <select class="form-select" bind:value={level} style="width: 12rem">
    <option value="">All Levels</option>
    <option value="INFO">INFO</option>
    <option value="WARN">WARN</option>
    <option value="ERROR">ERROR</option>
  </select>
  <input class="form-control" placeholder="Search logs" bind:value={search} />
  <button class="btn btn-warning text-dark" on:click={load}>Load</button>
</div>

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
