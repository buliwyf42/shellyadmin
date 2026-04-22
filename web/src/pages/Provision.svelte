<script lang="ts">
  import { onMount } from 'svelte';
  import { APIError, api } from '../lib/api';
  import type { Credential, CredentialGroup, Device, ProvisionResult } from '../lib/types';
  import ErrorNotice from '../components/ErrorNotice.svelte';
  import Select from '../components/Select.svelte';
  import SysForm from './provision/SysForm.svelte';
  import MqttForm from './provision/MqttForm.svelte';
  import WsForm from './provision/WsForm.svelte';
  import BleForm from './provision/BleForm.svelte';
  import MiscForm from './provision/MiscForm.svelte';
  import WifiAPForm from './provision/WifiAPForm.svelte';
  import EthForm from './provision/EthForm.svelte';
  import ModbusForm from './provision/ModbusForm.svelte';
  import ZigbeeForm from './provision/ZigbeeForm.svelte';
  import UserCAForm from './provision/UserCAForm.svelte';
  import ScriptsForm from './provision/ScriptsForm.svelte';
  import type {
    AuthState,
    BleState,
    CloudState,
    EthState,
    MatterState,
    ModbusState,
    MqttState,
    OtaState,
    ScriptsState,
    SysState,
    UIState,
    WifiAPState,
    WifiState,
    WsState,
    ZigbeeState,
  } from './provision/types';
  import {
    buildAuth,
    buildBle,
    buildCloud,
    buildEth,
    buildMatter,
    buildModbus,
    buildMqtt,
    buildOta,
    buildScripts,
    buildSys,
    buildUI,
    buildWifi,
    buildWifiAP,
    buildWs,
    buildZigbee,
    createAuthState,
    createBleState,
    createCloudState,
    createEthState,
    createMatterState,
    createModbusState,
    createMqttState,
    createOtaState,
    createScriptsState,
    createSysState,
    createUIState,
    createWifiAPState,
    createWifiState,
    createWsState,
    createZigbeeState,
    hydrateAuth,
    hydrateBle,
    hydrateCloud,
    hydrateEth,
    hydrateMatter,
    hydrateModbus,
    hydrateMqtt,
    hydrateOta,
    hydrateScripts,
    hydrateSys,
    hydrateUI,
    hydrateWifi,
    hydrateWifiAP,
    hydrateWs,
    hydrateZigbee,
  } from './provision/state';

  type PrecheckIssue = { ip: string; label: string; reason: string; category: 'auth' | 'other' };

  let devices: Device[] = [];
  let selected = new Set<string>();
  let loading = false;
  let running = false;
  let error = '';
  let errorDetails = '';
  let results: ProvisionResult[] = [];
  let templateNames: string[] = [];
  let credentials: Credential[] = [];
  let credentialGroups: CredentialGroup[] = [];
  let deviceGroupAssignments: Record<string, string> = {};
  let selectedTemplate = '';
  let selectedTemplateCredentialRef = '';
  let autoSelectedCredentialRef = '';
  let templateName = '';
  let viewMode: 'form' | 'json' = 'form';
  let advancedModeEnabled = false;
  let jsonText = '{}';
  let templateLoadNotice = '';
  let copiedSkipped = false;

  let sysState: SysState = createSysState();
  let mqttState: MqttState = createMqttState();
  let wsState: WsState = createWsState();
  let bleState: BleState = createBleState();
  let matterState: MatterState = createMatterState();
  let cloudState: CloudState = createCloudState();
  let otaState: OtaState = createOtaState();
  let authState: AuthState = createAuthState();
  let wifiState: WifiState = createWifiState();
  let wifiAPState: WifiAPState = createWifiAPState();
  let ethState: EthState = createEthState();
  let modbusState: ModbusState = createModbusState();
  let zigbeeState: ZigbeeState = createZigbeeState();
  let uiState: UIState = createUIState();
  let scriptsState: ScriptsState = createScriptsState();

  function captureError(err: unknown) {
    if (err instanceof APIError) {
      error = err.message;
      errorDetails = `${err.method} ${err.path} -> ${err.status}\n${JSON.stringify(err.detail ?? {}, null, 2)}`;
      return;
    }
    error = (err as Error).message;
    errorDetails = String(err);
  }

  function clearTemplateLoadNotice() {
    templateLoadNotice = '';
  }

  onMount(async () => {
    loading = true;
    error = '';
    try {
      const [
        loadedDevices,
        loadedTemplates,
        loadedCredentialGroups,
        loadedGroupAssignments,
        loadedSettings,
      ] = await Promise.all([
        api.getDevices(),
        api.listTemplates(),
        api.listCredentialGroups(),
        api.getCredentialGroupAssignments(),
        api.getSettings(),
      ]);
      devices = loadedDevices;
      templateNames = loadedTemplates;
      credentials = await api.listCredentials();
      credentialGroups = loadedCredentialGroups;
      deviceGroupAssignments = loadedGroupAssignments.assignments;
      advancedModeEnabled = loadedSettings.advanced_mode_enabled;
      if (!advancedModeEnabled) viewMode = 'form';
    } catch (err) {
      captureError(err);
    } finally {
      loading = false;
    }
    jsonText = JSON.stringify(buildTemplate(), null, 2);
  });

  function toggle(mac: string, checked: boolean) {
    if (checked) selected.add(mac);
    else selected.delete(mac);
    selected = new Set(selected);
  }

  function selectAll() {
    selected = new Set(devices.map((d) => d.mac));
  }

  function selectNone() {
    selected = new Set();
  }

  function selectedDevices() {
    return devices.filter((d) => selected.has(d.mac));
  }

  function templateForPrecheck(): Record<string, unknown> | null {
    if (viewMode !== 'json') return buildTemplate();
    try {
      return JSON.parse(jsonText) as Record<string, unknown>;
    } catch {
      return null;
    }
  }

  function reasonBadgeClass(category: PrecheckIssue['category']): string {
    switch (category) {
      case 'auth':
        return 'bg-warning text-dark';
      default:
        return 'bg-secondary';
    }
  }

  function reasonBadgeText(category: PrecheckIssue['category']): string {
    switch (category) {
      case 'auth':
        return 'auth';
      default:
        return 'other';
    }
  }

  function selectOnlyEligible() {
    if (!precheckTemplate || precheckTemplateError) return;
    const skippedIPs = new Set(precheckIssues.map((issue) => issue.ip));
    selected = new Set(
      selectedDevices()
        .filter((device) => !skippedIPs.has(device.ip))
        .map((device) => device.mac),
    );
  }

  async function copySkippedIPs() {
    const ips = [...new Set(precheckIssues.map((issue) => issue.ip))];
    if (ips.length === 0) return;
    try {
      await navigator.clipboard.writeText(ips.join('\n'));
      copiedSkipped = true;
      setTimeout(() => {
        copiedSkipped = false;
      }, 1500);
    } catch {
      copiedSkipped = false;
    }
  }

  $: precheckTemplate = templateForPrecheck();
  $: precheckTemplateError =
    viewMode === 'json' && precheckTemplate === null
      ? 'JSON is invalid; precheck is disabled until JSON is valid.'
      : '';
  $: groupCredentialByName = Object.fromEntries(
    credentialGroups.map((group) => [group.name, group.name]),
  );
  $: groupResolution = (() => {
    const chosenDevices = selectedDevices();
    let unresolved = 0;
    const credentials = new Set<string>();
    for (const device of chosenDevices) {
      const groupName = deviceGroupAssignments[device.mac];
      if (!groupName) {
        unresolved++;
        continue;
      }
      const credentialRef = groupCredentialByName[groupName];
      if (!credentialRef) {
        unresolved++;
        continue;
      }
      credentials.add(credentialRef);
    }
    return {
      total: chosenDevices.length,
      unresolved,
      credentialRefs: [...credentials],
    };
  })();
  $: groupCredentialHint = (() => {
    if (groupResolution.total === 0) return '';
    if (groupResolution.credentialRefs.length === 1 && groupResolution.unresolved === 0) {
      return `Credential defaulted from device groups: ${groupResolution.credentialRefs[0]}`;
    }
    if (groupResolution.credentialRefs.length > 1) {
      return 'Selected devices resolve to multiple group credentials. Choose a credential manually.';
    }
    if (groupResolution.unresolved > 0) {
      return `${groupResolution.unresolved} selected device(s) have no resolvable credential group.`;
    }
    return '';
  })();
  $: resolvedGroupCredentialRef =
    groupResolution.credentialRefs.length === 1 && groupResolution.unresolved === 0
      ? groupResolution.credentialRefs[0]
      : '';
  $: if (resolvedGroupCredentialRef) {
    if (
      !selectedTemplateCredentialRef ||
      selectedTemplateCredentialRef === autoSelectedCredentialRef
    ) {
      selectedTemplateCredentialRef = resolvedGroupCredentialRef;
      autoSelectedCredentialRef = resolvedGroupCredentialRef;
    }
  } else if (
    autoSelectedCredentialRef &&
    selectedTemplateCredentialRef === autoSelectedCredentialRef
  ) {
    selectedTemplateCredentialRef = '';
    autoSelectedCredentialRef = '';
  }
  $: templateOptions = templateNames.map((name) => ({ value: name, label: name }));
  $: credentialOptions = [
    { value: '', label: 'No credential', description: 'Skip auth for selected devices' },
    ...credentials.map((credential) => ({ value: credential.name, label: credential.name })),
  ];
  $: precheckIssues = selectedDevices().flatMap((device): PrecheckIssue[] => {
    if (!precheckTemplate) return [];
    if (device.auth_required && !selectedTemplateCredentialRef.trim()) {
      return [
        {
          ip: device.ip,
          label: device.name || device.serial || device.mac,
          reason: 'auth required but no credential ref selected',
          category: 'auth',
        },
      ];
    }
    return [];
  });
  $: precheckEligibleCount = Math.max(0, selectedDevices().length - precheckIssues.length);
  $: precheckReasonCounts = precheckIssues.reduce(
    (acc, issue) => {
      acc[issue.category] = (acc[issue.category] || 0) + 1;
      return acc;
    },
    {} as Record<string, number>,
  );

  function resetFormState() {
    sysState = createSysState();
    mqttState = createMqttState();
    wsState = createWsState();
    bleState = createBleState();
    matterState = createMatterState();
    cloudState = createCloudState();
    otaState = createOtaState();
    authState = createAuthState();
    wifiState = createWifiState();
    wifiAPState = createWifiAPState();
    ethState = createEthState();
    modbusState = createModbusState();
    zigbeeState = createZigbeeState();
    uiState = createUIState();
    scriptsState = createScriptsState();
  }

  function asRecord(value: unknown): Record<string, unknown> | null {
    return value && typeof value === 'object' && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : null;
  }

  function hydrateFormFromTemplate(
    template: Record<string, unknown>,
  ): { ok: true } | { ok: false; reason: string } {
    let nextSys: SysState | null = null;
    let nextMqtt: MqttState | null = null;
    let nextWs: WsState | null = null;
    let nextBle: BleState | null = null;
    let nextMatter: MatterState | null = null;
    let nextCloud: CloudState | null = null;
    let nextOta: OtaState | null = null;
    let nextAuth: AuthState | null = null;
    let nextWifi: WifiState | null = null;
    let nextWifiAP: WifiAPState | null = null;
    let nextEth: EthState | null = null;
    let nextModbus: ModbusState | null = null;
    let nextZigbee: ZigbeeState | null = null;
    let nextUI: UIState | null = null;
    let nextScripts: ScriptsState | null = null;
    for (const [sectionName, rawSection] of Object.entries(template)) {
      const section = sectionName.trim().toLowerCase();
      const record = asRecord(rawSection);
      if (!record) {
        return { ok: false, reason: `Template section "${sectionName}" is not an object.` };
      }
      switch (section) {
        case 'sys': {
          const r = hydrateSys(record);
          if (!r.ok) return r;
          nextSys = r.state;
          break;
        }
        case 'mqtt': {
          const r = hydrateMqtt(record);
          if (!r.ok) return r;
          nextMqtt = r.state;
          break;
        }
        case 'ws': {
          const r = hydrateWs(record);
          if (!r.ok) return r;
          nextWs = r.state;
          break;
        }
        case 'ble': {
          const r = hydrateBle(record);
          if (!r.ok) return r;
          nextBle = r.state;
          break;
        }
        case 'matter': {
          const r = hydrateMatter(record);
          if (!r.ok) return r;
          nextMatter = r.state;
          break;
        }
        case 'cloud': {
          const r = hydrateCloud(record);
          if (!r.ok) return r;
          nextCloud = r.state;
          break;
        }
        case 'ota': {
          const r = hydrateOta(record);
          if (!r.ok) return r;
          nextOta = r.state;
          break;
        }
        case 'auth': {
          const r = hydrateAuth(record);
          if (!r.ok) return r;
          nextAuth = r.state;
          break;
        }
        case 'wifi': {
          const r = hydrateWifi(record);
          if (!r.ok) return r;
          nextWifi = r.state;
          const ap = hydrateWifiAP(record);
          if (!ap.ok) return ap;
          nextWifiAP = ap.state;
          break;
        }
        case 'eth': {
          const r = hydrateEth(record);
          if (!r.ok) return r;
          nextEth = r.state;
          break;
        }
        case 'modbus': {
          const r = hydrateModbus(record);
          if (!r.ok) return r;
          nextModbus = r.state;
          break;
        }
        case 'zigbee': {
          const r = hydrateZigbee(record);
          if (!r.ok) return r;
          nextZigbee = r.state;
          break;
        }
        case 'ui': {
          const r = hydrateUI(record);
          if (!r.ok) return r;
          nextUI = r.state;
          break;
        }
        case 'script': {
          const r = hydrateScripts(record);
          if (!r.ok) return r;
          nextScripts = r.state;
          break;
        }
        default:
          return {
            ok: false,
            reason: `Template section "${sectionName}" is not supported by the form editor.`,
          };
      }
    }
    resetFormState();
    if (nextSys) sysState = nextSys;
    if (nextMqtt) mqttState = nextMqtt;
    if (nextWs) wsState = nextWs;
    if (nextBle) bleState = nextBle;
    if (nextMatter) matterState = nextMatter;
    if (nextCloud) cloudState = nextCloud;
    if (nextOta) otaState = nextOta;
    if (nextAuth) authState = nextAuth;
    if (nextWifi) wifiState = nextWifi;
    if (nextWifiAP) wifiAPState = nextWifiAP;
    if (nextEth) ethState = nextEth;
    if (nextModbus) modbusState = nextModbus;
    if (nextZigbee) zigbeeState = nextZigbee;
    if (nextUI) uiState = nextUI;
    if (nextScripts) scriptsState = nextScripts;
    return { ok: true };
  }

  function buildTemplate() {
    const out: Record<string, unknown> = {};
    const sys = buildSys(sysState);
    if (sys) out.sys = sys;
    const mqtt = buildMqtt(mqttState);
    if (mqtt) out.mqtt = mqtt;
    const ws = buildWs(wsState);
    if (ws) out.ws = ws;
    const ble = buildBle(bleState);
    if (ble) out.ble = ble;
    const matter = buildMatter(matterState);
    if (matter) out.matter = matter;
    const cloud = buildCloud(cloudState);
    if (cloud) out.cloud = cloud;
    const ota = buildOta(otaState);
    if (ota) out.ota = ota;
    const auth = buildAuth(authState);
    if (auth) out.auth = auth;
    const wifi = buildWifi(wifiState);
    const wifiAP = buildWifiAP(wifiAPState);
    if (wifi || wifiAP) {
      out.wifi = { ...(wifi ?? {}), ...(wifiAP ? { ap: wifiAP } : {}) };
    }
    const eth = buildEth(ethState);
    if (eth) out.eth = eth;
    const modbus = buildModbus(modbusState);
    if (modbus) out.modbus = modbus;
    const zigbee = buildZigbee(zigbeeState);
    if (zigbee) out.zigbee = zigbee;
    const ui = buildUI(uiState);
    if (ui) out.ui = ui;
    const scripts = buildScripts(scriptsState);
    if (scripts) out.script = scripts;
    return out;
  }

  function syncJSONFromForm() {
    jsonText = JSON.stringify(buildTemplate(), null, 2);
  }

  function setView(mode: 'form' | 'json') {
    if (mode === 'json') syncJSONFromForm();
    viewMode = mode;
  }

  async function saveCurrentTemplate() {
    if (!templateName.trim()) {
      error = 'Template name is required';
      return;
    }
    try {
      const body = viewMode === 'json' ? jsonText : JSON.stringify(buildTemplate(), null, 2);
      await api.saveTemplate(templateName.trim(), body, selectedTemplateCredentialRef);
      templateNames = await api.listTemplates();
      selectedTemplate = templateName.trim();
      error = '';
      errorDetails = '';
    } catch (err) {
      captureError(err);
    }
  }

  async function deleteCurrentTemplate() {
    if (!selectedTemplate) return;
    const name = selectedTemplate;
    try {
      await api.deleteTemplate(name);
      templateNames = await api.listTemplates();
      selectedTemplate = '';
      templateName = '';
      error = '';
      errorDetails = '';
    } catch (err) {
      captureError(err);
    }
  }

  async function renameCurrentTemplate() {
    const oldName = selectedTemplate;
    const newName = templateName.trim();
    if (!oldName || !newName || oldName === newName) return;
    try {
      const body = viewMode === 'json' ? jsonText : JSON.stringify(buildTemplate(), null, 2);
      await api.saveTemplate(newName, body, selectedTemplateCredentialRef);
      await api.deleteTemplate(oldName);
      templateNames = await api.listTemplates();
      selectedTemplate = newName;
      error = '';
      errorDetails = '';
    } catch (err) {
      captureError(err);
    }
  }

  async function loadCurrentTemplate() {
    if (!selectedTemplate) return;
    try {
      const loaded = await api.getTemplate(selectedTemplate);
      jsonText = loaded.content;
      selectedTemplateCredentialRef = loaded.credential_ref || '';
      templateName = selectedTemplate;
      clearTemplateLoadNotice();
      const parsed = asRecord(JSON.parse(loaded.content));
      const hydrated = parsed
        ? hydrateFormFromTemplate(parsed)
        : {
            ok: false as const,
            reason: 'Template root is not an object and cannot be represented in the form.',
          };
      if (hydrated.ok) {
        viewMode = 'form';
      } else {
        viewMode = 'json';
        templateLoadNotice = `Loaded in JSON mode: ${hydrated.reason}`;
      }
      error = '';
      errorDetails = '';
    } catch (err) {
      captureError(err);
    }
  }

  async function runProvision() {
    running = true;
    error = '';
    errorDetails = '';
    try {
      const template = viewMode === 'json' ? JSON.parse(jsonText) : buildTemplate();
      results = await api.provision(
        selectedDevices().map((device) => device.ip),
        template,
        selectedTemplateCredentialRef,
      );
    } catch (err) {
      captureError(err);
    } finally {
      running = false;
    }
  }
</script>

<ErrorNotice summary={error} details={errorDetails} />

<div class="row g-3">
  <div class="col-lg-6 provision-devices-col">
    {#if selected.size > 0}
      <div class="card bg-dark border-secondary">
        <div class="card-body">
          <h2 class="h6">Precheck Summary</h2>
          {#if precheckTemplateError}
            <div class="alert alert-warning py-2 mb-2">{precheckTemplateError}</div>
          {/if}
          <p class="mb-2">
            <span class="badge bg-success me-2">{precheckEligibleCount}</span> eligible
          </p>
          <p class="mb-2">
            <span class="badge bg-warning text-dark me-2">{precheckIssues.length}</span> predicted to
            be skipped
          </p>
          <div class="d-flex gap-2 flex-wrap mb-2">
            <button
              class="btn btn-sm btn-outline-light"
              on:click={selectOnlyEligible}
              disabled={precheckIssues.length === 0 || Boolean(precheckTemplateError)}
              >Select Only Eligible</button
            >
            <button
              class="btn btn-sm btn-outline-light"
              on:click={copySkippedIPs}
              disabled={precheckIssues.length === 0}>Copy Skipped IPs</button
            >
            {#if copiedSkipped}
              <span class="badge bg-success">copied</span>
            {/if}
            {#if precheckReasonCounts.auth}
              <span class="badge bg-warning text-dark">auth: {precheckReasonCounts.auth}</span>
            {/if}
            {#if precheckReasonCounts.generation}
              <span class="badge bg-info text-dark"
                >generation: {precheckReasonCounts.generation}</span
              >
            {/if}
          </div>
          {#if precheckIssues.length > 0}
            <div class="table-responsive">
              <table class="table table-dark table-striped table-sm mb-0">
                <thead>
                  <tr><th>IP</th><th>Device</th><th>Type</th><th>Reason</th></tr>
                </thead>
                <tbody>
                  {#each precheckIssues as issue}
                    <tr>
                      <td>{issue.ip}</td>
                      <td>{issue.label}</td>
                      <td
                        ><span class={`badge ${reasonBadgeClass(issue.category)}`}
                          >{reasonBadgeText(issue.category)}</span
                        ></td
                      >
                      <td>{issue.reason}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>
      </div>
    {/if}

    <div class="card bg-dark border-secondary">
      <div class="card-header d-flex justify-content-between align-items-center">
        <span>Select Devices</span>
        <div class="d-flex gap-2">
          <button class="btn btn-sm btn-outline-light" on:click={selectAll}>All</button>
          <button class="btn btn-sm btn-outline-light" on:click={selectNone}>None</button>
        </div>
      </div>
      <div class="card-body p-0">
        {#if loading}
          <div class="p-2 text-secondary">Loading devices...</div>
        {:else if devices.length === 0}
          <div class="p-2 text-secondary">No devices enrolled yet.</div>
        {:else}
          <div class="table-responsive device-list-scroll">
            <table class="table table-dark table-striped align-middle table-nowrap mb-0">
              <thead>
                <tr>
                  <th></th>
                  <th>IP</th>
                  <th>Name</th>
                  <th>Gen</th>
                </tr>
              </thead>
              <tbody>
                {#each devices as device}
                  <tr>
                    <td
                      ><input
                        type="checkbox"
                        class="form-check-input"
                        checked={selected.has(device.mac)}
                        on:change={(e) =>
                          toggle(device.mac, (e.currentTarget as HTMLInputElement).checked)}
                      /></td
                    >
                    <td>{device.ip}</td>
                    <td>{device.name || device.serial || device.mac}</td>
                    <td><span class="badge bg-success">Gen{device.gen}</span></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
      <div class="card-footer p-2 text-secondary">{selected.size} of {devices.length} selected</div>
    </div>
  </div>

  <div class="col-lg-6 provision-settings-col">
    <div class="card bg-dark border-secondary">
      <div class="card-header">
        <div class="provision-toolbar">
          <div class="sa-cluster">
            <div>
              <span class="sa-cluster-label">Template</span>
              <div class="sa-cluster-inner">
                <Select
                  bind:value={selectedTemplate}
                  options={templateOptions}
                  placeholder="Select a template…"
                  ariaLabel="Load template"
                />
                <button
                  class="btn btn-sm btn-outline-light"
                  on:click={loadCurrentTemplate}
                  disabled={!selectedTemplate}>Load</button
                >
                <button
                  class="btn btn-sm btn-outline-danger"
                  on:click={deleteCurrentTemplate}
                  disabled={!selectedTemplate}>Delete</button
                >
              </div>
            </div>
          </div>
          <div class="sa-cluster">
            <div>
              <span class="sa-cluster-label">Save as</span>
              <div class="sa-cluster-inner">
                <input class="form-control" placeholder="template name" bind:value={templateName} />
                <button class="btn btn-sm btn-outline-light" on:click={saveCurrentTemplate}
                  >Save</button
                >
                <button
                  class="btn btn-sm btn-outline-secondary"
                  on:click={renameCurrentTemplate}
                  disabled={!selectedTemplate ||
                    !templateName.trim() ||
                    selectedTemplate === templateName.trim()}>Rename</button
                >
              </div>
            </div>
          </div>
          <div class="sa-cluster">
            <div>
              <span class="sa-cluster-label">Credential</span>
              <div class="sa-cluster-inner">
                <Select
                  bind:value={selectedTemplateCredentialRef}
                  options={credentialOptions}
                  placeholder="No credential"
                  ariaLabel="Credential"
                />
              </div>
            </div>
          </div>
          {#if advancedModeEnabled}
            <div class="sa-cluster-spacer"></div>
            <div class="sa-view-switch" role="group" aria-label="View mode">
              <button
                type="button"
                class:is-active={viewMode === 'form'}
                on:click={() => setView('form')}>Form</button
              >
              <button
                type="button"
                class:is-active={viewMode === 'json'}
                on:click={() => setView('json')}>JSON</button
              >
            </div>
          {/if}
        </div>
        {#if groupCredentialHint}
          <div class="text-secondary mt-2" style="font-size: 0.82rem;">{groupCredentialHint}</div>
        {/if}
      </div>

      <div class="card-body">
        {#if templateLoadNotice}
          <div class="alert alert-info py-2">{templateLoadNotice}</div>
        {/if}
        {#if viewMode === 'json'}
          <textarea class="form-control font-monospace" rows="36" bind:value={jsonText}></textarea>
        {:else}
          <div class="d-flex flex-column gap-3">
            <SysForm bind:state={sysState} />
            <MqttForm bind:state={mqttState} />
            <WsForm bind:state={wsState} />
            <BleForm bind:state={bleState} />
            <MiscForm
              bind:matter={matterState}
              bind:cloud={cloudState}
              bind:ota={otaState}
              bind:auth={authState}
              bind:wifi={wifiState}
              bind:ui={uiState}
            />
            <WifiAPForm bind:state={wifiAPState} />
            <EthForm bind:state={ethState} />
            <ModbusForm bind:state={modbusState} />
            <ZigbeeForm bind:state={zigbeeState} />
            <ScriptsForm bind:state={scriptsState} />
            <UserCAForm {devices} {selected} />
          </div>
        {/if}

        <div class="d-flex gap-2 mt-3 flex-wrap">
          <button
            class="btn btn-warning text-dark"
            on:click={runProvision}
            disabled={selected.size === 0 || running}
            >{running ? 'Provisioning...' : `Provision ${selected.size}`}</button
          >
          {#if advancedModeEnabled}
            <button
              class="btn btn-outline-light"
              on:click={syncJSONFromForm}
              disabled={viewMode !== 'form'}>Sync JSON</button
            >
          {/if}
        </div>
      </div>
    </div>
  </div>
</div>

{#if results.length}
  {@const restartRequiredDevices = results.filter((r) => r.restart_required)}
  <div class="card bg-dark border-secondary mt-3" role="status" aria-live="polite">
    <div class="card-body">
      <div class="d-flex align-items-center justify-content-between mb-3 flex-wrap gap-2">
        <h2 class="h5 mb-0">Results</h2>
        {#if restartRequiredDevices.length > 0}
          <button
            class="btn btn-sm btn-warning"
            disabled={running}
            on:click={async () => {
              const macs = restartRequiredDevices
                .map((r) => devices.find((d) => d.ip === r.info.ip)?.mac)
                .filter(Boolean) as string[];
              if (!macs.length) return;
              if (!confirm(`Reboot ${macs.length} device(s) that require a restart?`)) return;
              try {
                await api.bulk({ action: 'reboot', macs });
              } catch (err) {
                captureError(err);
              }
            }}
            >Reboot {restartRequiredDevices.length} restart-required device{restartRequiredDevices.length !==
            1
              ? 's'
              : ''}</button
          >
        {/if}
      </div>
      <div class="table-responsive">
        <table class="table table-dark table-sm align-middle mb-0">
          <thead>
            <tr>
              <th>IP</th>
              <th>Device</th>
              <th>Status</th>
              <th>Sections</th>
            </tr>
          </thead>
          <tbody>
            {#each results as r}
              {@const overallOk = r.results.every(
                (s) => s.status === 'ok' || s.status === 'skipped',
              )}
              {@const hasFailed = r.results.some((s) => s.status === 'failed')}
              <tr>
                <td class="text-monospace small">{r.info.ip}</td>
                <td class="small">{r.info.name || r.info.model || '—'}</td>
                <td>
                  {#if hasFailed}
                    <span class="badge bg-danger">failed</span>
                  {:else if overallOk}
                    <span class="badge bg-success">ok</span>
                  {:else}
                    <span class="badge bg-secondary">partial</span>
                  {/if}
                  {#if r.restart_required}
                    <span class="badge ms-1" style="background:#c89a2a;color:#fff;"
                      >restart required</span
                    >
                  {/if}
                </td>
                <td>
                  <div class="d-flex flex-wrap gap-1">
                    {#each r.results as s}
                      <span
                        class="badge {s.status === 'ok'
                          ? 'bg-success'
                          : s.status === 'skipped'
                            ? 'bg-secondary'
                            : 'bg-danger'}"
                        title={s.detail}>{s.section}</span
                      >
                    {/each}
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  </div>
{/if}
