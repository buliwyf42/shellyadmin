<script lang="ts">
  import type { AuthState, CloudState, MatterState, OtaState, WifiState } from './types'

  export let matter: MatterState
  export let cloud: CloudState
  export let ota: OtaState
  export let auth: AuthState
  export let wifi: WifiState

  $: matterExpanded = matter.enabled || matter.enableField
  $: cloudExpanded = cloud.enabled || cloud.enableField
  $: otaExpanded = ota.enabled || ota.stageEnabled || ota.autoUpdateEnabled
  $: authExpanded = auth.enabled || auth.passEnabled
  $: wifiExpanded = wifi.enabled || wifi.staEnabled || wifi.ssidEnabled || wifi.passEnabled

  $: matterVisible = matterExpanded || matter.open
  $: cloudVisible = cloudExpanded || cloud.open
  $: otaVisible = otaExpanded || ota.open
  $: authVisible = authExpanded || auth.open
  $: wifiVisible = wifiExpanded || wifi.open
</script>

<div class="card bg-black border-secondary">
  <div class="card-body">
    <div
      class="d-flex justify-content-between align-items-center mb-3"
      role="button"
      tabindex="0"
      on:click={() => (matter.open = !matter.open)}
      on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (matter.open = !matter.open)}
      style="cursor: pointer"
    >
      <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer">
        <input type="checkbox" class="form-check-input" bind:checked={matter.enabled} on:click|stopPropagation />
        <strong>matter</strong> - Matter (Gen 2+)
      </label>
      <span class="text-secondary">{matterVisible ? '▾' : '▸'}</span>
    </div>
    {#if matterVisible}
      <div class="row g-2">
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={matter.enableField} disabled={!matter.enabled} />
            Enable Matter
          </label>
          <select class="form-select" bind:value={matter.enable} disabled={!matter.enabled || !matter.enableField}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
      </div>
    {/if}
  </div>
</div>

<div class="card bg-black border-secondary">
  <div class="card-body">
    <div
      class="d-flex justify-content-between align-items-center mb-3"
      role="button"
      tabindex="0"
      on:click={() => (cloud.open = !cloud.open)}
      on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (cloud.open = !cloud.open)}
      style="cursor: pointer"
    >
      <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer">
        <input type="checkbox" class="form-check-input" bind:checked={cloud.enabled} on:click|stopPropagation />
        <strong>cloud</strong> - Shelly Cloud
      </label>
      <span class="text-secondary">{cloudVisible ? '▾' : '▸'}</span>
    </div>
    {#if cloudVisible}
      <div class="row g-2">
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={cloud.enableField} disabled={!cloud.enabled} />
            Enable Cloud
          </label>
          <select class="form-select" bind:value={cloud.enable} disabled={!cloud.enabled || !cloud.enableField}>
            <option value={true}>On</option>
            <option value={false}>Off</option>
          </select>
        </div>
      </div>
    {/if}
  </div>
</div>

<div class="card bg-black border-secondary">
  <div class="card-body">
    <div
      class="d-flex justify-content-between align-items-center mb-3"
      role="button"
      tabindex="0"
      on:click={() => (ota.open = !ota.open)}
      on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (ota.open = !ota.open)}
      style="cursor: pointer"
    >
      <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer">
        <input type="checkbox" class="form-check-input" bind:checked={ota.enabled} on:click|stopPropagation />
        <strong>ota</strong> - Firmware Update
      </label>
      <span class="text-secondary">{otaVisible ? '▾' : '▸'}</span>
    </div>
    {#if otaVisible}
      <div class="row g-2">
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={ota.stageEnabled} disabled={!ota.enabled} />
            Stage
          </label>
          <select class="form-select" bind:value={ota.stage} disabled={!ota.enabled || !ota.stageEnabled}>
            <option value="stable">Stable</option>
            <option value="beta">Beta</option>
          </select>
        </div>
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={ota.autoUpdateEnabled} disabled={!ota.enabled} />
            Update automatically
          </label>
          <select class="form-select" bind:value={ota.autoUpdate} disabled={!ota.enabled || !ota.autoUpdateEnabled}>
            <option value="off">Disable auto update</option>
            <option value="stable">Enable update to stable version</option>
            <option value="beta">Enable update to beta version</option>
          </select>
          <div class="text-secondary mt-2">BETA firmware may cause instability</div>
        </div>
      </div>
    {/if}
  </div>
</div>

<div class="card bg-black border-secondary">
  <div class="card-body">
    <div
      class="d-flex justify-content-between align-items-center mb-3"
      role="button"
      tabindex="0"
      on:click={() => (auth.open = !auth.open)}
      on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (auth.open = !auth.open)}
      style="cursor: pointer"
    >
      <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer">
        <input type="checkbox" class="form-check-input" bind:checked={auth.enabled} on:click|stopPropagation />
        <strong>auth</strong> - Set Device Password (Gen 2+)
      </label>
      <span class="text-secondary">{authVisible ? '▾' : '▸'}</span>
    </div>
    {#if authVisible}
      <div class="row g-2">
        <div class="col-md-6">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={auth.passEnabled} disabled={!auth.enabled} />
            Password
          </label>
          <input class="form-control" type="password" bind:value={auth.pass} disabled={!auth.enabled || !auth.passEnabled} />
        </div>
      </div>
    {/if}
  </div>
</div>

<div class="card bg-black border-secondary">
  <div class="card-body">
    <div
      class="d-flex justify-content-between align-items-center mb-3"
      role="button"
      tabindex="0"
      on:click={() => (wifi.open = !wifi.open)}
      on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (wifi.open = !wifi.open)}
      style="cursor: pointer"
    >
      <label class="d-flex align-items-center gap-2 mb-0" style="cursor: pointer">
        <input type="checkbox" class="form-check-input" bind:checked={wifi.enabled} on:click|stopPropagation />
        <strong>wifi</strong> - WiFi STA
      </label>
      <span class="text-secondary">{wifiVisible ? '▾' : '▸'}</span>
    </div>
    {#if wifiVisible}
      <div class="row g-2">
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={wifi.staEnabled} disabled={!wifi.enabled} />
            Enable STA
          </label>
          <div class="text-secondary">On when section selected</div>
        </div>
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={wifi.ssidEnabled} disabled={!wifi.enabled} />
            SSID
          </label>
          <input class="form-control" bind:value={wifi.ssid} disabled={!wifi.enabled || !wifi.ssidEnabled} />
        </div>
        <div class="col-md-4">
          <label class="d-flex gap-2">
            <input type="checkbox" class="form-check-input" bind:checked={wifi.passEnabled} disabled={!wifi.enabled} />
            Password
          </label>
          <input class="form-control" type="password" bind:value={wifi.pass} disabled={!wifi.enabled || !wifi.passEnabled} />
        </div>
      </div>
    {/if}
  </div>
</div>
