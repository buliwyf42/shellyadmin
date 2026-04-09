<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import { devices } from '../lib/stores'
  import type { AppSettings, Device } from '../lib/types'
  import ComplianceBadge from '../components/ComplianceBadge.svelte'

  let settings: AppSettings = { subnets: [], scan_timeout: 2, scan_concurrency: 64, compliance: {} }
  let saved = ''
  let loading = false
  let error = ''

  async function load() {
    loading = true
    error = ''
    const [settingsResult, devicesResult] = await Promise.allSettled([
      api.getSettings(),
      api.getDevices(),
    ])

    if (settingsResult.status === 'fulfilled') {
      settings = settingsResult.value
    } else {
      error = settingsResult.reason instanceof Error ? settingsResult.reason.message : 'Failed to load compliance settings'
    }

    if (devicesResult.status === 'fulfilled') {
      $devices = devicesResult.value
    } else {
      error = error || (devicesResult.reason instanceof Error ? devicesResult.reason.message : 'Failed to load devices')
      $devices = []
    }
    loading = false
  }

  async function save() {
    await api.saveSettings(settings)
    await load()
    saved = 'Saved'
    setTimeout(() => saved = '', 1500)
  }

  $: compliantDevices = $devices.filter((device: Device) => device.compliant)
  $: nonCompliantDevices = $devices.filter((device: Device) => !device.compliant)
  $: sortedDevices = [...$devices].sort((a, b) => (a.name || a.serial || a.mac).localeCompare(b.name || b.serial || b.mac))

  onMount(() => void load())
</script>

<div class="row g-3">
  <div class="col-lg-8">
    <div class="card bg-dark border-secondary">
      <div class="card-body">
        <h2 class="h5">Compliance Rules</h2>
        <div class="row g-3">
          <div class="col-md-6"><label class="form-label" for="compliance-wifi-ssid">WiFi SSID</label><input id="compliance-wifi-ssid" class="form-control" bind:value={settings.compliance.wifi_ssid} /></div>
          <div class="col-md-6"><label class="form-label" for="compliance-mqtt-server">MQTT Server</label><input id="compliance-mqtt-server" class="form-control" bind:value={settings.compliance.mqtt_server} /></div>
          <div class="col-md-6"><label class="form-label" for="compliance-mqtt-client-id">MQTT Client ID</label><input id="compliance-mqtt-client-id" class="form-control" bind:value={settings.compliance.mqtt_client_id} /></div>
          <div class="col-md-6"><label class="form-label" for="compliance-topic-prefix">Topic Prefix</label><input id="compliance-topic-prefix" class="form-control" bind:value={settings.compliance.mqtt_topic_prefix} /></div>
          <div class="col-md-4"><label class="form-label" for="compliance-timezone">Timezone</label><input id="compliance-timezone" class="form-control" bind:value={settings.compliance.tz} /></div>
          <div class="col-md-4"><label class="form-label" for="compliance-sntp-server">SNTP Server</label><input id="compliance-sntp-server" class="form-control" bind:value={settings.compliance.sntp_server} /></div>
          <div class="col-md-4"><label class="form-label" for="compliance-time-format">Time Format</label><select id="compliance-time-format" class="form-select" bind:value={settings.compliance.time_format}><option value="">Ignore</option><option value="24h">24h</option><option value="12h">12h</option></select></div>
        </div>
        <button class="btn btn-warning text-dark mt-3" on:click={save}>Save Compliance</button>
        {#if saved}<span class="ms-2 text-success">{saved}</span>{/if}
      </div>
    </div>
  </div>
  <div class="col-lg-4">
    <div class="card bg-dark border-info">
      <div class="card-body">
        <h2 class="h6">Summary</h2>
        <p class="mb-2"><span class="badge bg-success me-2">{compliantDevices.length}</span> compliant</p>
        <p class="mb-2"><span class="badge bg-danger me-2">{nonCompliantDevices.length}</span> non-compliant</p>
        <p class="text-secondary mb-2">Use <code class="font-monospace">{'{device_name}'}</code> in Client ID or Topic Prefix values for per-device substitutions during provisioning.</p>
        <p class="text-secondary mb-0">Gen1 devices connected to Shelly Cloud skip MQTT compliance checks.</p>
      </div>
    </div>
  </div>
</div>

{#if error}
  <div class="alert alert-danger mt-3">{error}</div>
{/if}

<div class="card bg-dark border-secondary mt-3">
  <div class="card-body">
    <h2 class="h5">Compliance Results</h2>
    {#if loading}
      <div class="text-secondary">Loading compliance results...</div>
    {:else if sortedDevices.length === 0}
      <div class="alert alert-secondary mb-0">No enrolled devices available for compliance checks yet.</div>
    {:else}
      <div class="table-responsive">
        <table class="table table-dark table-striped align-middle">
          <thead>
            <tr>
              <th>Name</th>
              <th>IP</th>
              <th>Type</th>
              <th>Status</th>
              <th>Issues</th>
            </tr>
          </thead>
          <tbody>
            {#each sortedDevices as device}
              <tr>
                <td>{device.name || device.serial || device.mac}</td>
                <td><a href={`http://${device.ip}`} target="_blank" rel="noreferrer" class="text-decoration-none">{device.ip}</a></td>
                <td>{device.gen <= 1 ? 'Gen 1.x' : `Gen ${device.gen}.x`}</td>
                <td><ComplianceBadge compliant={device.compliant} issues={device.compliance_issues} /></td>
                <td>{device.compliance_issues?.join(', ') || 'No issues'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</div>
