<script lang="ts">
  import { onMount } from 'svelte'
  import { APIError, api } from '../lib/api'
  import ErrorNotice from '../components/ErrorNotice.svelte'

  let spec: Record<string, unknown> | null = null
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

  onMount(async () => {
    try {
      spec = await api.getOpenAPIV1()
    } catch (err) {
      captureError(err)
    }
  })
</script>

<section class="page-hero mb-3">
  <div class="page-hero-stack">
    <span class="page-kicker">API Docs</span>
    <h1 class="h4 mb-0">Documented v1 integration surface</h1>
  </div>
</section>

<ErrorNotice summary={error} details={errorDetails} />

<div class="row g-3">
  <div class="col-lg-5">
    <div class="card bg-dark border-secondary">
      <div class="card-body">
        <h2 class="h5">What is supported</h2>
        <ul class="mb-0">
          <li>Session/login endpoints, runtime metadata, inventory, device detail, and action discovery</li>
          <li>Scan, refresh, firmware, provisioning, and bulk settings workflows including `dry_run` previews</li>
          <li>Settings, templates, credentials, auth groups, logs, backup export/import, and the OpenAPI document itself</li>
        </ul>
      </div>
    </div>
  </div>
  <div class="col-lg-7">
    <div class="card bg-dark border-secondary">
      <div class="card-body">
        <h2 class="h5">OpenAPI JSON</h2>
        <p class="text-secondary">Fetch this directly from <code>/api/openapi/v1.json</code> for the current documented v1 route set used by tooling and automation.</p>
        <pre class="mb-0 raw-block">{JSON.stringify(spec ?? {}, null, 2)}</pre>
      </div>
    </div>
  </div>
</div>
<style>
  .raw-block {
    max-height: 34rem;
    overflow: auto;
    padding: 0.75rem;
    border-radius: 0.6rem;
    background: rgba(0, 0, 0, 0.25);
  }
</style>
