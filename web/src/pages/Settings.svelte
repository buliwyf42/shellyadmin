<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import type { AppSettings } from '../lib/types'

  let settings: AppSettings = { subnets: [], scan_timeout: 2, scan_concurrency: 64, compliance: {} }
  let templateNames: string[] = []
  let newTemplateName = ''
  let newTemplateContent = '{\n  "mqtt": {\n    "enable": true\n  }\n}'

  async function load() {
    settings = await api.getSettings()
    templateNames = await api.listTemplates()
  }

  async function saveSettings() {
    await api.saveSettings(settings)
  }

  async function createTemplate() {
    await api.saveTemplate(newTemplateName, newTemplateContent)
    newTemplateName = ''
    await load()
  }

  async function removeTemplate(name: string) {
    await api.deleteTemplate(name)
    await load()
  }

  onMount(() => void load())
</script>

<div class="row g-3">
  <div class="col-lg-6">
    <div class="card bg-dark border-secondary">
      <div class="card-body">
        <h2 class="h5">Scan Settings</h2>
        <label class="form-label">Subnets (one per line)</label>
        <textarea class="form-control mb-3" rows="6" value={settings.subnets.join('\n')} on:input={(e) => settings.subnets = (e.currentTarget as HTMLTextAreaElement).value.split('\n').map((v) => v.trim()).filter(Boolean)}></textarea>
        <div class="row g-3">
          <div class="col-md-6"><label class="form-label">Timeout (s)</label><input class="form-control" type="number" bind:value={settings.scan_timeout} /></div>
          <div class="col-md-6"><label class="form-label">Concurrency</label><input class="form-control" type="number" bind:value={settings.scan_concurrency} /></div>
        </div>
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
        <textarea class="form-control font-monospace mb-2" rows="8" bind:value={newTemplateContent}></textarea>
        <button class="btn btn-outline-light" on:click={createTemplate} disabled={!newTemplateName}>Create Template</button>
      </div>
    </div>
  </div>
</div>
