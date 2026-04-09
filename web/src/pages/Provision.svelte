<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import type { Device } from '../lib/types'

  type ProvisionResult = { info: unknown; results: unknown[] }
  type AdvancedRPC = { method: string; payload: string }
  type AdvancedHTTP = { endpoint: string; params: string }

  let devices: Device[] = []
  let selected = new Set<string>()
  let results: ProvisionResult[] = []
  let loading = false
  let running = false
  let error = ''
  let mode: 'guided' | 'advanced' = 'guided'
  let preview = '{}'
  let previewError = ''

  let sysEnabled = true
  let sysName = ''
  let sysTZ = 'Europe/Berlin'
  let sysSNTP = 'pool.ntp.org'
  let sysLat = '52.5200'
  let sysLon = '13.4050'
  let sysTimeFormat: '24h' | '12h' = '24h'
  let sysEco = false
  let sysDiscoverable = true

  let mqttEnabled = true
  let mqttEnable = true
  let mqttServer = ''
  let mqttClientID = 'shelly-{device_name}'
  let mqttTopicPrefix = 'shelly/{device_name}'
  let mqttUser = ''
  let mqttPass = ''
  let mqttRPCNtf = true
  let mqttStatusNtf = true
  let mqttEnableRPC = true
  let mqttEnableControl = true

  let cloudEnabled = false
  let cloudEnable = true

  let wsEnabled = false
  let wsEnable = false
  let wsServer = ''

  let bleEnabled = false
  let bleGateway = false

  let wifiEnabled = false
  let wifiSSID = ''
  let wifiPass = ''

  let authEnabled = false
  let authPass = ''

  let otaEnabled = false
  let otaStage: 'stable' | 'beta' = 'stable'

  let advancedRPC: AdvancedRPC[] = [{ method: 'Shelly.SetConfig', payload: '{\n  "config": {}\n}' }]
  let advancedHTTP: AdvancedHTTP[] = [{ endpoint: 'settings/mqtt', params: '{\n  "enable": true\n}' }]

  onMount(async () => {
    loading = true
    error = ''
    try {
      devices = await api.getDevices()
    } catch (err) {
      error = (err as Error).message
    } finally {
      loading = false
    }
  })

  function toggle(mac: string, checked: boolean) {
    if (checked) selected.add(mac)
    else selected.delete(mac)
    selected = new Set(selected)
  }

  function selectedDevices() {
    return devices.filter((device) => selected.has(device.mac))
  }

  function selectedGen1Only(): boolean {
    const picked = selectedDevices()
    return picked.length > 0 && picked.every((device) => device.gen <= 1)
  }

  function selectedGen2Only(): boolean {
    const picked = selectedDevices()
    return picked.length > 0 && picked.every((device) => device.gen >= 2)
  }

  function addRPC() {
    advancedRPC = [...advancedRPC, { method: '', payload: '{\n}' }]
  }

  function addHTTP() {
    advancedHTTP = [...advancedHTTP, { endpoint: '', params: '{\n}' }]
  }

  function removeRPC(index: number) {
    advancedRPC = advancedRPC.filter((_, i) => i !== index)
  }

  function removeHTTP(index: number) {
    advancedHTTP = advancedHTTP.filter((_, i) => i !== index)
  }

  function buildTemplate() {
    const template: Record<string, unknown> = {}

    if (sysEnabled) {
      template.sys = {
        name: sysName || undefined,
        tz: sysTZ || undefined,
        lat: Number(sysLat),
        lon: Number(sysLon),
        lng: Number(sysLon),
        clock_mode: sysTimeFormat === '12h' ? 1 : 0,
        location: {
          tz: sysTZ || undefined,
          lat: Number(sysLat),
          lon: Number(sysLon),
        },
        sntp: { server: sysSNTP || undefined },
        device: {
          eco_mode: sysEco,
          discoverable: sysDiscoverable,
        },
      }
    }

    if (mqttEnabled) {
      template.mqtt = {
        enable: mqttEnable,
        server: mqttServer || undefined,
        client_id: mqttClientID || undefined,
        topic_prefix: mqttTopicPrefix || undefined,
        user: mqttUser || undefined,
        pass: mqttPass || undefined,
        id: mqttClientID || undefined,
        rpc_ntf: mqttRPCNtf,
        status_ntf: mqttStatusNtf,
        enable_rpc: mqttEnableRPC,
        enable_control: mqttEnableControl,
      }
    }

    if (cloudEnabled) template.cloud = { enable: cloudEnable }
    if (wsEnabled) template.ws = { enable: wsEnable, server: wsServer || undefined }
    if (bleEnabled) template.ble = { gateway: { enable: bleGateway } }
    if (wifiEnabled) template.wifi = { sta: { enable: true, ssid: wifiSSID, pass: wifiPass } }
    if (authEnabled) template.auth = { pass: authPass }
    if (otaEnabled) template.ota = { stage: otaStage }

    if (mode === 'advanced') {
      const gen2RPC: Record<string, unknown> = {}
      for (const row of advancedRPC) {
        if (!row.method.trim()) continue
        gen2RPC[row.method.trim()] = parseJSON(row.payload || '{}', `Gen2 RPC ${row.method}`)
      }
      if (Object.keys(gen2RPC).length > 0) template.gen2_rpc = gen2RPC

      const gen1HTTP: Record<string, unknown> = {}
      for (const row of advancedHTTP) {
        if (!row.endpoint.trim()) continue
        gen1HTTP[row.endpoint.trim()] = parseJSON(row.params || '{}', `Gen1 endpoint ${row.endpoint}`)
      }
      if (Object.keys(gen1HTTP).length > 0) template.gen1_http = gen1HTTP
    }

    return template
  }

  function parseJSON(raw: string, label: string) {
    try {
      return JSON.parse(raw)
    } catch (err) {
      throw new Error(`${label} has invalid JSON: ${(err as Error).message}`)
    }
  }

  $: {
    try {
      preview = JSON.stringify(buildTemplate(), null, 2)
      previewError = ''
    } catch (err) {
      preview = '{}'
      previewError = (err as Error).message
    }
  }

  async function runProvision() {
    running = true
    error = ''
    try {
      results = await api.provision(
        selectedDevices().map((device) => device.ip),
        buildTemplate(),
      )
    } catch (err) {
      error = (err as Error).message
    } finally {
      running = false
    }
  }
</script>

<div class="row g-3">
  <div class="col-lg-4">
    <div class="card bg-dark border-secondary">
      <div class="card-body">
        <h2 class="h5">Targets</h2>
        {#if loading}
          <div class="text-secondary">Loading devices...</div>
        {:else if devices.length === 0}
          <div class="alert alert-secondary mb-0">No enrolled devices yet.</div>
        {:else}
          {#each devices as device}
            <label class="d-flex gap-2 mb-2 align-items-center">
              <input type="checkbox" class="form-check-input" on:change={(e) => toggle(device.mac, (e.currentTarget as HTMLInputElement).checked)} />
              <span>{device.name || device.serial || device.mac}</span>
              <span class={`badge ${device.gen <= 1 ? 'bg-danger' : 'bg-success'}`}>{device.gen <= 1 ? 'Gen 1.x' : `Gen ${device.gen}.x`}</span>
            </label>
          {/each}
        {/if}
      </div>
    </div>
    <div class="card bg-dark border-info mt-3">
      <div class="card-body">
        <h2 class="h6">Compatibility</h2>
        <p class="mb-2 text-secondary">Gen1 and Gen2+ are provisioned differently. Unsupported sections are skipped with result details.</p>
        <p class="mb-2"><span class="badge bg-danger me-2">Gen1</span> HTTP endpoints (`/settings`, `/settings/mqtt`)</p>
        <p class="mb-0"><span class="badge bg-success me-2">Gen2+</span> RPC methods (`*.SetConfig`, `Shelly.SetAuth`, `Shelly.Update`)</p>
      </div>
    </div>
  </div>

  <div class="col-lg-8">
    <div class="card bg-dark border-secondary">
      <div class="card-body">
        <div class="d-flex justify-content-between align-items-center mb-3">
          <h2 class="h5 mb-0">Provisioning Template</h2>
          <div class="btn-group d-flex gap-2">
            <button type="button" class={`btn btn-sm ${mode === 'guided' ? 'btn-warning text-dark' : 'btn-outline-light'}`} on:click={() => mode = 'guided'}>Guided</button>
            <button type="button" class={`btn btn-sm ${mode === 'advanced' ? 'btn-warning text-dark' : 'btn-outline-light'}`} on:click={() => mode = 'advanced'}>Advanced</button>
          </div>
        </div>

        <div class="row g-3 mb-3">
          <div class="col-md-6">
            <label class="d-flex align-items-center gap-2 mb-2"><input type="checkbox" class="form-check-input" bind:checked={sysEnabled} /> System</label>
            <input class="form-control mb-2" placeholder="Name (optional)" bind:value={sysName} disabled={!sysEnabled} />
            <input class="form-control mb-2" placeholder="Timezone" bind:value={sysTZ} disabled={!sysEnabled} />
            <input class="form-control mb-2" placeholder="SNTP Server" bind:value={sysSNTP} disabled={!sysEnabled} />
            <div class="d-flex gap-2">
              <input class="form-control" placeholder="Lat" bind:value={sysLat} disabled={!sysEnabled} />
              <input class="form-control" placeholder="Lon" bind:value={sysLon} disabled={!sysEnabled} />
            </div>
            <select class="form-select mt-2" bind:value={sysTimeFormat} disabled={!sysEnabled}>
              <option value="24h">24h</option>
              <option value="12h">12h</option>
            </select>
            <label class="d-flex align-items-center gap-2 mt-2"><input type="checkbox" class="form-check-input" bind:checked={sysEco} disabled={!sysEnabled} /> Eco Mode</label>
            <label class="d-flex align-items-center gap-2"><input type="checkbox" class="form-check-input" bind:checked={sysDiscoverable} disabled={!sysEnabled} /> Discoverable</label>
          </div>

          <div class="col-md-6">
            <label class="d-flex align-items-center gap-2 mb-2"><input type="checkbox" class="form-check-input" bind:checked={mqttEnabled} /> MQTT</label>
            <label class="d-flex align-items-center gap-2 mb-2"><input type="checkbox" class="form-check-input" bind:checked={mqttEnable} disabled={!mqttEnabled} /> Enabled</label>
            <input class="form-control mb-2" placeholder="Broker host:port" bind:value={mqttServer} disabled={!mqttEnabled} />
            <input class="form-control mb-2" placeholder="Client ID" bind:value={mqttClientID} disabled={!mqttEnabled} />
            <input class="form-control mb-2" placeholder="Topic Prefix" bind:value={mqttTopicPrefix} disabled={!mqttEnabled} />
            <input class="form-control mb-2" placeholder="User" bind:value={mqttUser} disabled={!mqttEnabled} />
            <input class="form-control mb-2" placeholder="Password" bind:value={mqttPass} disabled={!mqttEnabled} />
            <label class="d-flex align-items-center gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttRPCNtf} disabled={!mqttEnabled} /> RPC Notifications</label>
            <label class="d-flex align-items-center gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttStatusNtf} disabled={!mqttEnabled} /> Status Notifications</label>
            <label class="d-flex align-items-center gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttEnableRPC} disabled={!mqttEnabled} /> Enable RPC</label>
            <label class="d-flex align-items-center gap-2"><input type="checkbox" class="form-check-input" bind:checked={mqttEnableControl} disabled={!mqttEnabled} /> Enable Control</label>
          </div>
        </div>

        <div class="row g-3 mb-3">
          <div class="col-md-3">
            <label class="d-flex align-items-center gap-2 mb-2"><input type="checkbox" class="form-check-input" bind:checked={cloudEnabled} /> Cloud</label>
            <label class="d-flex align-items-center gap-2"><input type="checkbox" class="form-check-input" bind:checked={cloudEnable} disabled={!cloudEnabled} /> Enabled</label>
          </div>
          <div class="col-md-3">
            <label class="d-flex align-items-center gap-2 mb-2"><input type="checkbox" class="form-check-input" bind:checked={wsEnabled} /> WebSocket</label>
            <label class="d-flex align-items-center gap-2"><input type="checkbox" class="form-check-input" bind:checked={wsEnable} disabled={!wsEnabled} /> Enabled</label>
            <input class="form-control mt-2" placeholder="ws://host/path" bind:value={wsServer} disabled={!wsEnabled} />
          </div>
          <div class="col-md-3">
            <label class="d-flex align-items-center gap-2 mb-2"><input type="checkbox" class="form-check-input" bind:checked={bleEnabled} /> BLE</label>
            <label class="d-flex align-items-center gap-2"><input type="checkbox" class="form-check-input" bind:checked={bleGateway} disabled={!bleEnabled} /> Gateway Enabled</label>
          </div>
          <div class="col-md-3">
            <label class="d-flex align-items-center gap-2 mb-2"><input type="checkbox" class="form-check-input" bind:checked={wifiEnabled} /> WiFi (STA)</label>
            <input class="form-control mb-2" placeholder="SSID" bind:value={wifiSSID} disabled={!wifiEnabled} />
            <input class="form-control" placeholder="Password" bind:value={wifiPass} disabled={!wifiEnabled} />
          </div>
        </div>

        <div class="row g-3 mb-3">
          <div class="col-md-6">
            <label class="d-flex align-items-center gap-2 mb-2"><input type="checkbox" class="form-check-input" bind:checked={authEnabled} /> Auth (Gen2+)</label>
            <input class="form-control" type="password" placeholder="Admin Password" bind:value={authPass} disabled={!authEnabled} />
          </div>
          <div class="col-md-6">
            <label class="d-flex align-items-center gap-2 mb-2"><input type="checkbox" class="form-check-input" bind:checked={otaEnabled} /> OTA Update</label>
            <select class="form-select" bind:value={otaStage} disabled={!otaEnabled}>
              <option value="stable">stable</option>
              <option value="beta">beta</option>
            </select>
          </div>
        </div>

        {#if mode === 'advanced'}
          <div class="card bg-black border-secondary mb-3">
            <div class="card-body">
              <h3 class="h6">Advanced Gen2+ RPC (all writable methods)</h3>
              {#each advancedRPC as row, idx}
                <div class="border rounded p-2 mb-2">
                  <input class="form-control mb-2 font-monospace" placeholder="Method e.g. MQTT.SetConfig" bind:value={row.method} />
                  <textarea class="form-control font-monospace mb-2" rows="5" bind:value={row.payload}></textarea>
                  <button class="btn btn-sm btn-outline-danger" on:click={() => removeRPC(idx)}>Remove</button>
                </div>
              {/each}
              <button class="btn btn-sm btn-outline-light" on:click={addRPC}>Add RPC Method</button>
            </div>
          </div>

          <div class="card bg-black border-secondary mb-3">
            <div class="card-body">
              <h3 class="h6">Advanced Gen1 HTTP (all writable endpoints)</h3>
              {#each advancedHTTP as row, idx}
                <div class="border rounded p-2 mb-2">
                  <input class="form-control mb-2 font-monospace" placeholder="Endpoint e.g. settings/mqtt" bind:value={row.endpoint} />
                  <textarea class="form-control font-monospace mb-2" rows="5" bind:value={row.params}></textarea>
                  <button class="btn btn-sm btn-outline-danger" on:click={() => removeHTTP(idx)}>Remove</button>
                </div>
              {/each}
              <button class="btn btn-sm btn-outline-light" on:click={addHTTP}>Add HTTP Endpoint</button>
            </div>
          </div>
        {/if}

        <h3 class="h6 mb-2">Template Preview</h3>
        <textarea class="form-control font-monospace" rows="14" readonly value={preview}></textarea>
        {#if previewError}<div class="alert alert-danger mt-2 mb-0">{previewError}</div>{/if}

        <div class="d-flex gap-2 mt-3 flex-wrap">
          <button class="btn btn-warning text-dark" on:click={runProvision} disabled={selected.size === 0 || running}>Provision {selected.size}</button>
          <span class="text-secondary">Selected profile: {selectedGen1Only() ? 'Gen1 only' : selectedGen2Only() ? 'Gen2+ only' : 'Mixed'}</span>
        </div>
      </div>
    </div>
  </div>
</div>

{#if error}
  <div class="alert alert-danger mt-3">{error}</div>
{/if}

{#if results.length}
  <div class="card bg-dark border-secondary mt-3">
    <div class="card-body">
      <h2 class="h5">Results</h2>
      <pre class="mb-0">{JSON.stringify(results, null, 2)}</pre>
    </div>
  </div>
{/if}
