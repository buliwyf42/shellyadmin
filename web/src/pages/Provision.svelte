<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import type { Device } from '../lib/types'

  let devices: Device[] = []
  let selected = new Set<string>()
  let template = `{
  "sys": {
    "location": { "tz": "Europe/Berlin", "lat": 52.52, "lon": 13.4 }
  },
  "mqtt": {
    "enable": true,
    "server": "192.168.1.10:1883",
    "client_id": "shelly-{device_name}"
  }
}`
  let results: Array<{ info: unknown; results: unknown[] }> = []

  onMount(async () => {
    devices = await api.getDevices()
  })

  async function runProvision() {
    results = await api.provision(
      devices.filter((d) => selected.has(d.mac)).map((d) => d.ip),
      JSON.parse(template),
    )
  }
</script>

<div class="row g-3">
  <div class="col-lg-4">
    <div class="card bg-dark border-secondary">
      <div class="card-body">
        <h2 class="h5">Targets</h2>
        {#each devices as device}
          <label class="d-flex gap-2 mb-2 align-items-center">
            <input type="checkbox" class="form-check-input" on:change={(e) => (e.currentTarget as HTMLInputElement).checked ? selected.add(device.mac) : selected.delete(device.mac)} />
            <span>{device.name || device.serial || device.mac}</span>
          </label>
        {/each}
      </div>
    </div>
  </div>
  <div class="col-lg-8">
    <div class="card bg-dark border-secondary">
      <div class="card-body">
        <h2 class="h5">Template</h2>
        <textarea class="form-control font-monospace" rows="16" bind:value={template}></textarea>
        <button class="btn btn-warning text-dark mt-3" on:click={runProvision} disabled={selected.size === 0}>Provision {selected.size}</button>
      </div>
    </div>
  </div>
</div>

{#if results.length}
  <div class="card bg-dark border-secondary mt-3">
    <div class="card-body">
      <h2 class="h5">Results</h2>
      <pre class="mb-0">{JSON.stringify(results, null, 2)}</pre>
    </div>
  </div>
{/if}
