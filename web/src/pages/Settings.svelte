<script lang="ts">
  import { onMount } from 'svelte'
  import { APIError, api } from '../lib/api'
  import type { AppSettings, BackupExport, Credential, ImportReport } from '../lib/types'
  import ErrorNotice from '../components/ErrorNotice.svelte'

  let settings: AppSettings = { subnets: [], scan_timeout: 2, refresh_timeout: 5, scan_concurrency: 64, enable_mdns: false, compliance: {} }
  let templateNames: string[] = []
  let newTemplateName = ''
  let newTemplateContent = '{\n  "mqtt": {\n    "enable": true\n  }\n}'
  let newTemplateCredentialRef = ''
  let credentials: Credential[] = []
  let error = ''
  let errorDetails = ''
  let status = ''
  let includeSecrets = false
  let importText = ''
  let importReport: ImportReport | null = null
  let pendingImport: BackupExport | null = null

  function captureError(err: unknown) {
    if (err instanceof APIError) {
      error = err.message
      errorDetails = `${err.method} ${err.path} -> ${err.status}\n${JSON.stringify(err.detail ?? {}, null, 2)}`
      return
    }
    error = (err as Error).message
    errorDetails = String(err)
  }

  function setStatus(message: string) {
    status = message
    setTimeout(() => {
      if (status === message) status = ''
    }, 2000)
  }

  function clearMessages() {
    error = ''
    errorDetails = ''
  }

  function downloadJSON(filename: string, payload: unknown) {
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename
    link.click()
    URL.revokeObjectURL(url)
  }

  async function load() {
    clearMessages()
    try {
      settings = await api.getSettings()
      templateNames = await api.listTemplates()
      credentials = await api.listCredentials()
    } catch (err) {
      captureError(err)
    }
  }

  async function saveSettings() {
    clearMessages()
    try {
      await api.saveSettings(settings)
      setStatus('Settings saved')
    } catch (err) {
      captureError(err)
    }
  }

  async function createTemplate() {
    clearMessages()
    try {
      await api.saveTemplate(newTemplateName, newTemplateContent, newTemplateCredentialRef)
      newTemplateName = ''
      newTemplateCredentialRef = ''
      await load()
      setStatus('Template saved')
    } catch (err) {
      captureError(err)
    }
  }

  async function removeTemplate(name: string) {
    clearMessages()
    try {
      await api.deleteTemplate(name)
      await load()
      setStatus('Template deleted')
    } catch (err) {
      captureError(err)
    }
  }

  async function exportBackup() {
    clearMessages()
    try {
      const confirm = includeSecrets ? 'export-plaintext-secrets' : ''
      const payload = await api.exportBackup(includeSecrets, confirm)
      const suffix = includeSecrets ? 'with-secrets' : 'redacted'
      downloadJSON(`shellyadmin-backup-${suffix}.json`, payload)
      setStatus(includeSecrets ? 'Exported with plaintext secrets' : 'Exported redacted backup')
    } catch (err) {
      captureError(err)
    }
  }

  async function dryRunImport() {
    clearMessages()
    importReport = null
    pendingImport = null
    try {
      const parsed = JSON.parse(importText) as BackupExport
      const report = await api.importBackup(parsed, false)
      importReport = report
      pendingImport = parsed
      setStatus('Dry-run completed')
    } catch (err) {
      captureError(err)
    }
  }

  async function applyImport() {
    if (!pendingImport) {
      error = 'Run dry-run first'
      errorDetails = ''
      return
    }
    clearMessages()
    try {
      importReport = await api.importBackup(pendingImport, true)
      await load()
      setStatus('Import applied')
    } catch (err) {
      captureError(err)
    }
  }

  onMount(() => void load())
</script>

<ErrorNotice summary={error} details={errorDetails} />
{#if status}
  <div class="alert alert-secondary">{status}</div>
{/if}

<div class="row g-3">
  <div class="col-lg-6">
    <div class="card bg-dark border-secondary">
      <div class="card-body">
        <h2 class="h5">Discovery & Refresh</h2>
        <label class="form-label" for="settings-subnets">Subnets (one per line)</label>
        <textarea id="settings-subnets" class="form-control mb-3" rows="6" value={settings.subnets.join('\n')} on:input={(e) => settings.subnets = (e.currentTarget as HTMLTextAreaElement).value.split('\n').map((v) => v.trim()).filter(Boolean)}></textarea>
        <div class="row g-3">
          <div class="col-md-4"><label class="form-label" for="settings-scan-timeout">Scan timeout (s)</label><input id="settings-scan-timeout" class="form-control" type="number" bind:value={settings.scan_timeout} /></div>
          <div class="col-md-4"><label class="form-label" for="settings-refresh-timeout">Refresh timeout (s)</label><input id="settings-refresh-timeout" class="form-control" type="number" bind:value={settings.refresh_timeout} /></div>
          <div class="col-md-4"><label class="form-label" for="settings-scan-concurrency">Concurrency</label><input id="settings-scan-concurrency" class="form-control" type="number" bind:value={settings.scan_concurrency} /></div>
        </div>
        <label class="d-flex gap-2 align-items-center mt-3">
          <input type="checkbox" class="form-check-input" bind:checked={settings.enable_mdns} />
          <span>Enable mDNS-assisted discovery</span>
        </label>
        <p class="text-secondary mt-2 mb-0">Tune discovery, refresh, and concurrency here. mDNS works best on hosts that can receive local multicast traffic; Docker setups may need host networking for reliable results.</p>
        <button class="btn btn-warning text-dark mt-3" on:click={saveSettings}>Save Settings</button>
      </div>
    </div>
  </div>
  <div class="col-lg-6">
    <div class="card bg-dark border-secondary">
      <div class="card-body">
        <h2 class="h5">Templates</h2>
        <div class="d-flex flex-column gap-2 mb-3">
          {#each templateNames as name}
            <div class="d-flex justify-content-between align-items-center border rounded p-2">
              <span>{name}</span>
              <button class="btn btn-sm btn-outline-danger" on:click={() => removeTemplate(name)}>Delete</button>
            </div>
          {/each}
        </div>
        <input class="form-control mb-2" placeholder="Template name" bind:value={newTemplateName} />
        <label class="form-label" for="template-credential-ref">Default credential for this template (optional)</label>
        <select id="template-credential-ref" class="form-select mb-2" bind:value={newTemplateCredentialRef}>
          <option value="">none</option>
          {#each credentials as credential}
            <option value={credential.name}>{credential.name}</option>
          {/each}
        </select>
        <p class="text-secondary mb-2">Used during provisioning when devices require authentication. If empty, no credential is auto-selected.</p>
        <textarea class="form-control font-monospace mb-2" rows="8" bind:value={newTemplateContent}></textarea>
        <button class="btn btn-outline-light" on:click={createTemplate} disabled={!newTemplateName}>Create Template</button>
      </div>
    </div>
  </div>
  <div class="col-lg-12">
    <div class="card bg-dark border-secondary">
      <div class="card-body">
        <h2 class="h5">Backup (Settings + Templates + Groups)</h2>
        <p class="text-secondary">Restore is a two-step flow: dry-run first, then apply.</p>
        <div class="d-flex gap-2 align-items-center flex-wrap mb-3">
          <label class="d-flex gap-2 align-items-center mb-0">
            <input type="checkbox" class="form-check-input" bind:checked={includeSecrets} />
            Include plaintext secrets (requires explicit confirmation)
          </label>
          <button class="btn btn-outline-light" on:click={exportBackup}>Export JSON</button>
        </div>

        <label class="form-label" for="backup-import-json">Import JSON</label>
        <textarea id="backup-import-json" class="form-control font-monospace mb-2" rows="10" bind:value={importText} placeholder="Paste exported backup JSON here"></textarea>
        <div class="d-flex gap-2 flex-wrap">
          <button class="btn btn-outline-light" on:click={dryRunImport} disabled={!importText.trim()}>Dry Run</button>
          <button class="btn btn-warning text-dark" on:click={applyImport} disabled={!pendingImport}>Apply Import</button>
        </div>

        {#if importReport}
          <div class="mt-3">
            <h3 class="h6">Import Report</h3>
            <pre class="mb-0">{JSON.stringify(importReport, null, 2)}</pre>
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>
