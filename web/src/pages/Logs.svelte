<script lang="ts">
  import { onMount } from 'svelte';
  import { api, triggerDownload } from '../lib/api';
  import type { LogEntry } from '../lib/types';

  let level = '';
  let search = '';
  let risk = '';
  let logs: LogEntry[] = [];
  let busy = false;
  let notice = '';
  let error = '';

  async function load() {
    error = '';
    logs = await api.getLogs(level, search, risk);
  }

  async function exportLogs(format: 'csv' | 'ndjson') {
    error = '';
    try {
      const blob = await api.exportLogs(level, search, risk, format);
      const stamp = new Date().toISOString().replace(/[-:]/g, '').split('.')[0] + 'Z';
      triggerDownload(`shellyadmin-logs-${stamp}.${format}`, blob);
    } catch (err) {
      error = (err as Error).message;
    }
  }

  async function clearAll() {
    const confirmed = window.confirm('Delete all logs? This cannot be undone.');
    if (!confirmed) return;
    busy = true;
    error = '';
    notice = '';
    try {
      const result = await api.clearLogs();
      notice = `Deleted ${result.deleted} log entries.`;
      await load();
    } catch (err) {
      error = (err as Error).message;
    } finally {
      busy = false;
    }
  }

  onMount(() => void load());
</script>

<section class="page-hero">
  <div class="page-hero-stack">
    <span class="page-kicker">Logs</span>
    <h1 class="h5 mb-0">Audit trail</h1>
  </div>
  <div class="page-toolbar">
    <select class="form-select toolbar-select-md" bind:value={level}>
      <option value="">All Levels</option>
      <option value="INFO">INFO</option>
      <option value="WARN">WARN</option>
      <option value="ERROR">ERROR</option>
    </select>
    <select
      class="form-select toolbar-select-md"
      bind:value={risk}
      title="Filter by catalog risk level on action-execution rows"
    >
      <option value="">All Risks</option>
      <option value="low">Low</option>
      <option value="medium">Medium</option>
      <option value="high">High</option>
    </select>
    <input class="form-control toolbar-input-lg" placeholder="Search logs" bind:value={search} />
    <button class="btn btn-warning text-dark" on:click={load}>Load</button>
    <button class="btn btn-sm btn-outline-light" on:click={() => exportLogs('csv')}
      >Export CSV</button
    >
    <button class="btn btn-sm btn-outline-light" on:click={() => exportLogs('ndjson')}
      >Export NDJSON</button
    >
    <button class="btn btn-outline-danger" on:click={clearAll} disabled={busy}
      >Delete All Logs</button
    >
  </div>
</section>

{#if notice}
  <div class="alert alert-success py-2">{notice}</div>
{/if}
{#if error}
  <div class="alert alert-danger py-2">{error}</div>
{/if}

<div class="table-responsive">
  <table class="table table-dark table-striped">
    <thead
      ><tr
        ><th>Timestamp</th><th>Level</th><th title="Catalog risk level on action-execution rows"
          >Risk</th
        ><th>Request</th><th>Message</th></tr
      ></thead
    >
    <tbody>
      {#each logs as log (log.id)}
        <tr>
          <td>{log.ts}</td>
          <td
            ><span
              class={`badge ${log.level === 'ERROR' ? 'bg-danger' : log.level === 'WARN' ? 'bg-warning text-dark' : log.level === 'DEBUG' ? 'bg-info text-dark' : 'bg-secondary'}`}
              >{log.level}</span
            ></td
          >
          <td>
            {#if log.risk_level === 'high'}<span class="badge bg-danger" title="high-risk action"
                >high</span
              >{:else if log.risk_level === 'medium'}<span
                class="badge bg-warning text-dark"
                title="medium-risk action">medium</span
              >{:else if log.risk_level === 'low'}<span
                class="badge bg-secondary"
                title="low-risk action">low</span
              >{:else}<span class="text-muted">—</span>{/if}
          </td>
          <td class="log-request-id"
            >{#if log.request_id}<code title={log.request_id}>{log.request_id}</code>{:else}<span
                class="text-muted">—</span
              >{/if}</td
          >
          <td>{log.message}</td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .log-request-id code {
    font-size: 0.8rem;
    color: var(--bs-info, #0dcaf0);
    background: transparent;
    padding: 0;
  }
</style>
