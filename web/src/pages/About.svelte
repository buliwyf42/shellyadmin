<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import type { VersionInfo } from '../lib/types'
  import { APP_VERSION } from '../lib/version'

  let backend: VersionInfo = { backend_version: 'unknown', commit: 'unknown' }
  let loadError = ''

  onMount(async () => {
    try {
      backend = await api.getVersion()
    } catch (err) {
      loadError = (err as Error).message
    }
  })
</script>

<div class="row justify-content-center">
  <div class="col-xl-7 col-lg-8">
    <div class="card bg-dark border-secondary">
      <div class="card-header">About</div>
      <div class="card-body">
        <h1 class="h4 mb-3">ShellyAdmin</h1>
        <p class="mb-3">Self-hosted device management for Shelly fleets.</p>
        {#if loadError}
          <div class="alert alert-warning py-2">{loadError}</div>
        {/if}
        <dl class="row mb-0">
          <dt class="col-sm-4 text-secondary">Frontend Version</dt>
          <dd class="col-sm-8"><span class="badge bg-warning text-dark">v{APP_VERSION}</span></dd>
          <dt class="col-sm-4 text-secondary">Backend Version</dt>
          <dd class="col-sm-8"><span class="badge bg-info text-dark">v{backend.backend_version || 'unknown'}</span></dd>
          <dt class="col-sm-4 text-secondary">Commit</dt>
          <dd class="col-sm-8"><code>{backend.commit || 'unknown'}</code></dd>
          <dt class="col-sm-4 text-secondary">Frontend Stack</dt>
          <dd class="col-sm-8">Svelte + Vite</dd>
          <dt class="col-sm-4 text-secondary">Backend Stack</dt>
          <dd class="col-sm-8">Go + Gin + SQLite</dd>
        </dl>
      </div>
    </div>
  </div>
</div>
