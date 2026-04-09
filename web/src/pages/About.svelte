<script lang="ts">
  import { APP_VERSION } from '../lib/version'
</script>

<div class="row justify-content-center">
  <div class="col-xl-7 col-lg-8">
    <div class="card bg-dark border-secondary">
      <div class="card-header">About</div>
      <div class="card-body">
        <h1 class="h4 mb-3">ShellyAdmin</h1>
        <p class="mb-3">Self-hosted device management for Shelly fleets.</p>
        <dl class="row mb-0">
          <dt class="col-sm-4 text-secondary">Version</dt>
          <dd class="col-sm-8"><span class="badge bg-warning text-dark">v{APP_VERSION}</span></dd>
          <dt class="col-sm-4 text-secondary">Frontend</dt>
          <dd class="col-sm-8">Svelte + Vite</dd>
          <dt class="col-sm-4 text-secondary">Backend</dt>
          <dd class="col-sm-8">Go + Gin + SQLite</dd>
        </dl>
      </div>
    </div>
  </div>
</div>
