<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import { devices } from '../lib/stores'
  import type { AppSettings, CustomRule, Device } from '../lib/types'
  import ComplianceBadge from '../components/ComplianceBadge.svelte'

  let settings: AppSettings = { subnets: [], scan_timeout: 2, scan_concurrency: 64, compliance: { custom_rules: [] } }
  let saved = ''
  let loading = false
  let error = ''

  const sourceOptions: Array<CustomRule['source']> = ['device', 'config', 'status']
  const opOptions: Array<CustomRule['op']> = ['eq', 'ne', 'contains', 'regex', 'exists']

  let wifiSSIDEnabled = false

  let mqttEnabledField = false
  let mqttServerEnabled = false
  let mqttClientIDEnabled = false
  let mqttTopicPrefixEnabled = false
  let mqttRPCNtfEnabled = false
  let mqttStatusNtfEnabled = false
  let mqttEnableRPCEnabled = false
  let mqttEnableControlEnabled = false

  let cloudConnectedEnabled = false

  let wsEnabledField = false
  let wsConnectedField = false
  let wsServerField = false

  let bleGWEnabledField = false

  let tzEnabled = false
  let sntpEnabled = false
  let timeFormatEnabled = false
  let latEnabled = false
  let lonEnabled = false
  let ecoEnabled = false
  let discoverableEnabled = false

  let wifiOpen = false
  let mqttOpen = false
  let cloudOpen = false
  let wsOpen = false
  let bleOpen = false
  let sysOpen = false
  let customOpen = false

  $: wifiExpanded = wifiSSIDEnabled
  $: mqttExpanded = mqttEnabledField || mqttServerEnabled || mqttClientIDEnabled || mqttTopicPrefixEnabled || mqttRPCNtfEnabled || mqttStatusNtfEnabled || mqttEnableRPCEnabled || mqttEnableControlEnabled
  $: cloudExpanded = cloudConnectedEnabled
  $: wsExpanded = wsEnabledField || wsConnectedField || wsServerField
  $: bleExpanded = bleGWEnabledField
  $: sysExpanded = tzEnabled || sntpEnabled || timeFormatEnabled || latEnabled || lonEnabled || ecoEnabled || discoverableEnabled
  $: customExpanded = (settings.compliance.custom_rules || []).length > 0

  $: wifiVisible = wifiExpanded || wifiOpen
  $: mqttVisible = mqttExpanded || mqttOpen
  $: cloudVisible = cloudExpanded || cloudOpen
  $: wsVisible = wsExpanded || wsOpen
  $: bleVisible = bleExpanded || bleOpen
  $: sysVisible = sysExpanded || sysOpen
  $: customVisible = customExpanded || customOpen

  function ensureDefaults() {
    settings.compliance = settings.compliance || {}
    settings.compliance.custom_rules = settings.compliance.custom_rules || []
    if (settings.compliance.mqtt_enabled === undefined) settings.compliance.mqtt_enabled = null
    if (settings.compliance.cloud_connected === undefined) settings.compliance.cloud_connected = null
    if (settings.compliance.ws_enabled === undefined) settings.compliance.ws_enabled = null
    if (settings.compliance.ws_connected === undefined) settings.compliance.ws_connected = null
    if (settings.compliance.ble_gw_enabled === undefined) settings.compliance.ble_gw_enabled = null
    if (settings.compliance.mqtt_rpc_ntf === undefined) settings.compliance.mqtt_rpc_ntf = null
    if (settings.compliance.mqtt_status_ntf === undefined) settings.compliance.mqtt_status_ntf = null
    if (settings.compliance.mqtt_enable_rpc === undefined) settings.compliance.mqtt_enable_rpc = null
    if (settings.compliance.mqtt_enable_control === undefined) settings.compliance.mqtt_enable_control = null
    if (settings.compliance.eco_mode === undefined) settings.compliance.eco_mode = null
    if (settings.compliance.discoverable === undefined) settings.compliance.discoverable = null
  }

  function initToggles() {
    ensureDefaults()
    wifiSSIDEnabled = Boolean(settings.compliance.wifi_ssid)

    mqttEnabledField = settings.compliance.mqtt_enabled !== null && settings.compliance.mqtt_enabled !== undefined
    mqttServerEnabled = Boolean(settings.compliance.mqtt_server)
    mqttClientIDEnabled = Boolean(settings.compliance.mqtt_client_id)
    mqttTopicPrefixEnabled = Boolean(settings.compliance.mqtt_topic_prefix)
    mqttRPCNtfEnabled = settings.compliance.mqtt_rpc_ntf !== null && settings.compliance.mqtt_rpc_ntf !== undefined
    mqttStatusNtfEnabled = settings.compliance.mqtt_status_ntf !== null && settings.compliance.mqtt_status_ntf !== undefined
    mqttEnableRPCEnabled = settings.compliance.mqtt_enable_rpc !== null && settings.compliance.mqtt_enable_rpc !== undefined
    mqttEnableControlEnabled = settings.compliance.mqtt_enable_control !== null && settings.compliance.mqtt_enable_control !== undefined

    cloudConnectedEnabled = settings.compliance.cloud_connected !== null && settings.compliance.cloud_connected !== undefined

    wsEnabledField = settings.compliance.ws_enabled !== null && settings.compliance.ws_enabled !== undefined
    wsConnectedField = settings.compliance.ws_connected !== null && settings.compliance.ws_connected !== undefined
    wsServerField = Boolean(settings.compliance.ws_server)

    bleGWEnabledField = settings.compliance.ble_gw_enabled !== null && settings.compliance.ble_gw_enabled !== undefined

    tzEnabled = Boolean(settings.compliance.tz)
    sntpEnabled = Boolean(settings.compliance.sntp_server)
    timeFormatEnabled = Boolean(settings.compliance.time_format)
    latEnabled = settings.compliance.lat !== null && settings.compliance.lat !== undefined
    lonEnabled = settings.compliance.lon !== null && settings.compliance.lon !== undefined
    ecoEnabled = settings.compliance.eco_mode !== null && settings.compliance.eco_mode !== undefined
    discoverableEnabled = settings.compliance.discoverable !== null && settings.compliance.discoverable !== undefined
  }

  function applyTogglesToSettings() {
    ensureDefaults()
    settings.compliance.wifi_ssid = wifiSSIDEnabled ? (settings.compliance.wifi_ssid || '') : ''

    settings.compliance.mqtt_enabled = mqttEnabledField ? Boolean(settings.compliance.mqtt_enabled) : null
    settings.compliance.mqtt_server = mqttServerEnabled ? (settings.compliance.mqtt_server || '') : ''
    settings.compliance.mqtt_client_id = mqttClientIDEnabled ? (settings.compliance.mqtt_client_id || '') : ''
    settings.compliance.mqtt_topic_prefix = mqttTopicPrefixEnabled ? (settings.compliance.mqtt_topic_prefix || '') : ''
    settings.compliance.mqtt_rpc_ntf = mqttRPCNtfEnabled ? Boolean(settings.compliance.mqtt_rpc_ntf) : null
    settings.compliance.mqtt_status_ntf = mqttStatusNtfEnabled ? Boolean(settings.compliance.mqtt_status_ntf) : null
    settings.compliance.mqtt_enable_rpc = mqttEnableRPCEnabled ? Boolean(settings.compliance.mqtt_enable_rpc) : null
    settings.compliance.mqtt_enable_control = mqttEnableControlEnabled ? Boolean(settings.compliance.mqtt_enable_control) : null

    settings.compliance.cloud_connected = cloudConnectedEnabled ? Boolean(settings.compliance.cloud_connected) : null

    settings.compliance.ws_enabled = wsEnabledField ? Boolean(settings.compliance.ws_enabled) : null
    settings.compliance.ws_connected = wsConnectedField ? Boolean(settings.compliance.ws_connected) : null
    settings.compliance.ws_server = wsServerField ? (settings.compliance.ws_server || '') : ''

    settings.compliance.ble_gw_enabled = bleGWEnabledField ? Boolean(settings.compliance.ble_gw_enabled) : null

    settings.compliance.tz = tzEnabled ? (settings.compliance.tz || '') : ''
    settings.compliance.sntp_server = sntpEnabled ? (settings.compliance.sntp_server || '') : ''
    settings.compliance.time_format = timeFormatEnabled ? (settings.compliance.time_format || '') : ''
    settings.compliance.lat = latEnabled ? settings.compliance.lat : null
    settings.compliance.lon = lonEnabled ? settings.compliance.lon : null
    settings.compliance.eco_mode = ecoEnabled ? Boolean(settings.compliance.eco_mode) : null
    settings.compliance.discoverable = discoverableEnabled ? Boolean(settings.compliance.discoverable) : null
  }

  async function load() {
    loading = true
    error = ''
    const [settingsResult, devicesResult] = await Promise.allSettled([
      api.getSettings(),
      api.getDevices(),
    ])

    if (settingsResult.status === 'fulfilled') {
      settings = settingsResult.value
      ensureDefaults()
      initToggles()
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
    applyTogglesToSettings()
    settings.compliance.custom_rules = (settings.compliance.custom_rules || []).filter((rule) => rule.path.trim() !== '')
    await api.saveSettings(settings)
    await load()
    saved = 'Saved'
    setTimeout(() => (saved = ''), 1500)
  }

  function addRule() {
    ensureDefaults()
    settings.compliance.custom_rules = [
      ...settings.compliance.custom_rules!,
      {
        label: '',
        source: 'config',
        path: '',
        op: 'eq',
        value: '',
        gen_min: 0,
        gen_max: 0,
      },
    ]
  }

  function removeRule(index: number) {
    settings.compliance.custom_rules = (settings.compliance.custom_rules || []).filter((_, i) => i !== index)
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
        <p class="text-secondary mb-3">Same interaction style as Provision: section headers are clickable, fields are opt-in via checkboxes.</p>

        <div class="d-flex flex-column gap-3">
          <div class="card bg-black border-secondary">
            <div class="card-body">
              <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (wifiOpen = !wifiOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (wifiOpen = !wifiOpen)} style="cursor: pointer">
                <strong>wifi</strong>
                <span class="text-secondary">{wifiVisible ? '▾' : '▸'}</span>
              </div>
              {#if wifiVisible}
                <div class="row g-2">
                  <div class="col-md-6"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={wifiSSIDEnabled} on:click|stopPropagation />WiFi SSID</label><input class="form-control" bind:value={settings.compliance.wifi_ssid} disabled={!wifiSSIDEnabled} /></div>
                </div>
              {/if}
            </div>
          </div>

          <div class="card bg-black border-secondary">
            <div class="card-body">
              <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (mqttOpen = !mqttOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (mqttOpen = !mqttOpen)} style="cursor: pointer">
                <strong>mqtt</strong>
                <span class="text-secondary">{mqttVisible ? '▾' : '▸'}</span>
              </div>
              {#if mqttVisible}
                <div class="row g-2">
                  <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttEnabledField} on:click|stopPropagation />Enabled</label><select class="form-select" bind:value={settings.compliance.mqtt_enabled} disabled={!mqttEnabledField}><option value={true}>On</option><option value={false}>Off</option></select></div>
                  <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttServerEnabled} on:click|stopPropagation />Broker</label><input class="form-control" bind:value={settings.compliance.mqtt_server} disabled={!mqttServerEnabled} /></div>
                  <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttClientIDEnabled} on:click|stopPropagation />Client ID</label><input class="form-control" bind:value={settings.compliance.mqtt_client_id} disabled={!mqttClientIDEnabled} /></div>
                  <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttTopicPrefixEnabled} on:click|stopPropagation />Topic Prefix</label><input class="form-control" bind:value={settings.compliance.mqtt_topic_prefix} disabled={!mqttTopicPrefixEnabled} /></div>
                  <div class="col-md-2"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttRPCNtfEnabled} on:click|stopPropagation />rpc_ntf</label><select class="form-select" bind:value={settings.compliance.mqtt_rpc_ntf} disabled={!mqttRPCNtfEnabled}><option value={true}>On</option><option value={false}>Off</option></select></div>
                  <div class="col-md-2"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttStatusNtfEnabled} on:click|stopPropagation />status_ntf</label><select class="form-select" bind:value={settings.compliance.mqtt_status_ntf} disabled={!mqttStatusNtfEnabled}><option value={true}>On</option><option value={false}>Off</option></select></div>
                  <div class="col-md-2"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttEnableRPCEnabled} on:click|stopPropagation />enable_rpc</label><select class="form-select" bind:value={settings.compliance.mqtt_enable_rpc} disabled={!mqttEnableRPCEnabled}><option value={true}>On</option><option value={false}>Off</option></select></div>
                  <div class="col-md-2"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttEnableControlEnabled} on:click|stopPropagation />enable_control</label><select class="form-select" bind:value={settings.compliance.mqtt_enable_control} disabled={!mqttEnableControlEnabled}><option value={true}>On</option><option value={false}>Off</option></select></div>
                </div>
              {/if}
            </div>
          </div>

          <div class="card bg-black border-secondary">
            <div class="card-body">
              <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (cloudOpen = !cloudOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (cloudOpen = !cloudOpen)} style="cursor: pointer">
                <strong>cloud</strong>
                <span class="text-secondary">{cloudVisible ? '▾' : '▸'}</span>
              </div>
              {#if cloudVisible}
                <div class="row g-2">
                  <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={cloudConnectedEnabled} on:click|stopPropagation />Connected</label><select class="form-select" bind:value={settings.compliance.cloud_connected} disabled={!cloudConnectedEnabled}><option value={true}>Yes</option><option value={false}>No</option></select></div>
                </div>
              {/if}
            </div>
          </div>

          <div class="card bg-black border-secondary">
            <div class="card-body">
              <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (wsOpen = !wsOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (wsOpen = !wsOpen)} style="cursor: pointer">
                <strong>ws</strong>
                <span class="text-secondary">{wsVisible ? '▾' : '▸'}</span>
              </div>
              {#if wsVisible}
                <div class="row g-2">
                  <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={wsEnabledField} on:click|stopPropagation />Enabled</label><select class="form-select" bind:value={settings.compliance.ws_enabled} disabled={!wsEnabledField}><option value={true}>On</option><option value={false}>Off</option></select></div>
                  <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={wsConnectedField} on:click|stopPropagation />Connected</label><select class="form-select" bind:value={settings.compliance.ws_connected} disabled={!wsConnectedField}><option value={true}>Yes</option><option value={false}>No</option></select></div>
                  <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={wsServerField} on:click|stopPropagation />Server</label><input class="form-control" bind:value={settings.compliance.ws_server} disabled={!wsServerField} /></div>
                </div>
              {/if}
            </div>
          </div>

          <div class="card bg-black border-secondary">
            <div class="card-body">
              <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (bleOpen = !bleOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (bleOpen = !bleOpen)} style="cursor: pointer">
                <strong>ble</strong>
                <span class="text-secondary">{bleVisible ? '▾' : '▸'}</span>
              </div>
              {#if bleVisible}
                <div class="row g-2">
                  <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={bleGWEnabledField} on:click|stopPropagation />Gateway Enabled</label><select class="form-select" bind:value={settings.compliance.ble_gw_enabled} disabled={!bleGWEnabledField}><option value={true}>On</option><option value={false}>Off</option></select></div>
                </div>
              {/if}
            </div>
          </div>

          <div class="card bg-black border-secondary">
            <div class="card-body">
              <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (sysOpen = !sysOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (sysOpen = !sysOpen)} style="cursor: pointer">
                <strong>sys</strong>
                <span class="text-secondary">{sysVisible ? '▾' : '▸'}</span>
              </div>
              {#if sysVisible}
                <div class="row g-2">
                  <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={tzEnabled} on:click|stopPropagation />Timezone</label><input class="form-control" bind:value={settings.compliance.tz} disabled={!tzEnabled} /></div>
                  <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={sntpEnabled} on:click|stopPropagation />SNTP Server</label><input class="form-control" bind:value={settings.compliance.sntp_server} disabled={!sntpEnabled} /></div>
                  <div class="col-md-4"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={timeFormatEnabled} on:click|stopPropagation />Time Format</label><select class="form-select" bind:value={settings.compliance.time_format} disabled={!timeFormatEnabled}><option value="24h">24h</option><option value="12h">12h</option></select></div>
                  <div class="col-md-3"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={latEnabled} on:click|stopPropagation />Lat</label><input class="form-control" type="number" step="0.0001" bind:value={settings.compliance.lat} disabled={!latEnabled} /></div>
                  <div class="col-md-3"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={lonEnabled} on:click|stopPropagation />Lon</label><input class="form-control" type="number" step="0.0001" bind:value={settings.compliance.lon} disabled={!lonEnabled} /></div>
                  <div class="col-md-3"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={ecoEnabled} on:click|stopPropagation />Eco Mode</label><select class="form-select" bind:value={settings.compliance.eco_mode} disabled={!ecoEnabled}><option value={true}>On</option><option value={false}>Off</option></select></div>
                  <div class="col-md-3"><label class="d-flex gap-2"><input type="checkbox" class="form-check-input" bind:checked={discoverableEnabled} on:click|stopPropagation />Discoverable</label><select class="form-select" bind:value={settings.compliance.discoverable} disabled={!discoverableEnabled}><option value={true}>On</option><option value={false}>Off</option></select></div>
                </div>
              {/if}
            </div>
          </div>

          <div class="card bg-black border-secondary">
            <div class="card-body">
              <div class="d-flex justify-content-between align-items-center mb-3" role="button" tabindex="0" on:click={() => (customOpen = !customOpen)} on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (customOpen = !customOpen)} style="cursor: pointer">
                <strong>custom rules</strong>
                <span class="text-secondary">{customVisible ? '▾' : '▸'}</span>
              </div>
              {#if customVisible}
                <p class="text-secondary mb-2">source=`config|status|device`, path examples: `mqtt.server`, `sys.location.tz`, `cloud.connected`</p>
                {#each settings.compliance.custom_rules || [] as rule, idx}
                  <div class="border rounded p-2 mb-2">
                    <div class="row g-2">
                      <div class="col-md-3"><input class="form-control" placeholder="Label" bind:value={rule.label} /></div>
                      <div class="col-md-2"><select class="form-select" bind:value={rule.source}>{#each sourceOptions as option}<option value={option}>{option}</option>{/each}</select></div>
                      <div class="col-md-3"><input class="form-control font-monospace" placeholder="path.to.field" bind:value={rule.path} /></div>
                      <div class="col-md-2"><select class="form-select" bind:value={rule.op}>{#each opOptions as option}<option value={option}>{option}</option>{/each}</select></div>
                      <div class="col-md-2"><input class="form-control" placeholder="Expected value" bind:value={rule.value} disabled={rule.op === 'exists'} /></div>
                    </div>
                    <div class="row g-2 mt-2">
                      <div class="col-md-2"><input class="form-control" type="number" min="0" placeholder="Gen min" bind:value={rule.gen_min} /></div>
                      <div class="col-md-2"><input class="form-control" type="number" min="0" placeholder="Gen max" bind:value={rule.gen_max} /></div>
                      <div class="col-md-2"><button class="btn btn-sm btn-outline-danger" on:click={() => removeRule(idx)}>Remove</button></div>
                    </div>
                  </div>
                {/each}
                <button class="btn btn-sm btn-outline-light" on:click={addRule}>Add Rule</button>
              {/if}
            </div>
          </div>
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
        <p class="text-secondary mb-2">Token <code class="font-monospace">{'{device_name}'}</code> is substituted during provisioning.</p>
        <p class="text-secondary mb-0">Gen1 cloud-connected devices skip MQTT checks by design.</p>
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
        <table class="table table-dark table-striped align-middle table-nowrap">
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
