<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import type { VersionInfo } from '../lib/types'
  import { APP_VERSION } from '../lib/version'

  let backend: VersionInfo = { backend_version: 'unknown', commit: 'unknown' }
  let loadError = ''
  const projectURL = 'https://github.com/buliwyf42/shellyadmin'

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
    <div class="card bg-dark border-secondary about-card">
      <div class="card-header">About</div>
      <div class="card-body">
        <div class="about-hero">
          <div>
            <h1 class="h4 mb-2">ShellyAdmin</h1>
            <p class="mb-0 text-secondary">Self-hosted device management for trusted Shelly fleets.</p>
          </div>
          <a class="btn btn-outline-light" href={projectURL} target="_blank" rel="noreferrer">Project on GitHub</a>
        </div>
        {#if loadError}
          <div class="alert alert-warning py-2">{loadError}</div>
        {/if}
        <dl class="row mb-0 about-grid">
          <dt class="col-sm-4 text-secondary">Frontend Version</dt>
          <dd class="col-sm-8"><span class="badge bg-warning text-dark">v{APP_VERSION}</span></dd>
          <dt class="col-sm-4 text-secondary">Backend Version</dt>
          <dd class="col-sm-8"><span class="badge bg-info text-dark">v{backend.backend_version || 'unknown'}</span></dd>
          <dt class="col-sm-4 text-secondary">Commit</dt>
          <dd class="col-sm-8"><code>{backend.commit || 'unknown'}</code></dd>
          <dt class="col-sm-4 text-secondary">Project</dt>
          <dd class="col-sm-8"><a class="project-link" href={projectURL} target="_blank" rel="noreferrer">{projectURL}</a></dd>
          <dt class="col-sm-4 text-secondary">Frontend Stack</dt>
          <dd class="col-sm-8">Svelte + Vite</dd>
          <dt class="col-sm-4 text-secondary">Backend Stack</dt>
          <dd class="col-sm-8">Go + Gin + SQLite</dd>
        </dl>
      </div>
    </div>
  </div>
</div>

<style>
  .about-card {
    position: relative;
    overflow: hidden;
  }

  .about-card::before {
    content: '';
    position: absolute;
    inset: 0 auto auto 0;
    width: 100%;
    height: 0.2rem;
    background: linear-gradient(90deg, #f3c94c, #7ed6ad 58%, #6ec9ff);
  }

  .about-hero {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1rem;
    margin-bottom: 1.5rem;
  }

  .project-link {
    color: #78c7ff;
    text-decoration: none;
  }

  .project-link:hover,
  .project-link:focus-visible {
    color: #a7dcff;
  }

  @media (max-width: 720px) {
    .about-hero {
      flex-direction: column;
      align-items: stretch;
    }
  }
</style>
