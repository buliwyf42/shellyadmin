<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import type { AppSettings } from '../lib/types'

  let settings: AppSettings = { subnets: [], scan_timeout: 2, scan_concurrency: 64, compliance: {} }
  let saved = ''

  async function load() {
    settings = await api.getSettings()
  }

  async function save() {
    await api.saveSettings(settings)
    saved = 'Saved'
    setTimeout(() => saved = '', 1500)
  }

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
        <h2 class="h6">Notes</h2>
        <p class="text-secondary mb-2">Use `{device_name}` in Client ID or Topic Prefix values for per-device substitutions during provisioning.</p>
        <p class="text-secondary mb-0">Gen1 devices connected to Shelly Cloud skip MQTT compliance checks.</p>
      </div>
    </div>
  </div>
</div>
