<script lang="ts">
  import { onMount } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { api, toErrorDetails, toErrorMessage } from '../lib/api';
  import type { Credential, CredentialGroup, Device, ProvisionResult } from '../lib/types';
  import ErrorNotice from '../components/ErrorNotice.svelte';
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
  import CoverForm from './provision/CoverForm.svelte';
  import ScriptsForm from './provision/ScriptsForm.svelte';
  import WebhooksForm from './provision/WebhooksForm.svelte';
  import ZigbeeOpsForm from './provision/ZigbeeOpsForm.svelte';
  import ResultsPanel from './provision/ResultsPanel.svelte';
  import IPListPanel from './provision/IPListPanel.svelte';
  import TemplatesPanel from './provision/TemplatesPanel.svelte';
  import type {
    AuthState,
    AutoUpdateState,
    BleState,
    CloudState,
    CoverState,
    EthState,
    MatterState,
    ModbusState,
    MqttState,
    ScriptsState,
    SysState,
    UIState,
    WebhooksState,
    WifiAPState,
    WifiState,
    WsState,
    ZigbeeOpsState,
    ZigbeeState,
  } from './provision/types';
  import {
    buildAuth,
    buildAutoUpdate,
    buildBle,
    buildCloud,
    buildCover,
    buildEth,
    buildMatter,
    buildModbus,
    buildMqtt,
    buildScripts,
    buildSys,
    buildUI,
    buildWebhooks,
    buildWifi,
    buildWifiAP,
    buildWs,
    buildZigbee,
    buildZigbeeOps,
    createAuthState,
    createAutoUpdateState,
    createBleState,
    createCloudState,
    createCoverState,
    createEthState,
    createMatterState,
    createModbusState,
    createMqttState,
    createScriptsState,
    createSysState,
    createUIState,
    createWebhooksState,
    createWifiAPState,
    createWifiState,
    createWsState,
    createZigbeeOpsState,
    createZigbeeState,
    hydrateAuth,
    hydrateAutoUpdate,
    hydrateBle,
    hydrateCloud,
    hydrateCover,
    hydrateEth,
    hydrateMatter,
    hydrateModbus,
    hydrateMqtt,
    hydrateScripts,
    hydrateSys,
    hydrateUI,
    hydrateWebhooks,
    hydrateWifi,
    hydrateWifiAP,
    hydrateWs,
    hydrateZigbee,
  } from './provision/state';

  type PrecheckIssue = { ip: string; label: string; reason: string; category: 'auth' | 'other' };

  let devices: Device[] = [];
  let selected = new SvelteSet<string>();
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
  let autoUpdateState: AutoUpdateState = createAutoUpdateState();
  let cloudState: CloudState = createCloudState();
  let authState: AuthState = createAuthState();
  let wifiState: WifiState = createWifiState();
  let wifiAPState: WifiAPState = createWifiAPState();
  let ethState: EthState = createEthState();
  let modbusState: ModbusState = createModbusState();
  let zigbeeState: ZigbeeState = createZigbeeState();
  let uiState: UIState = createUIState();
  let scriptsState: ScriptsState = createScriptsState();
  let webhooksState: WebhooksState = createWebhooksState();
  let coverState: CoverState = createCoverState();
  let zigbeeOpsState: ZigbeeOpsState = createZigbeeOpsState();

  function captureError(err: unknown) {
    error = toErrorMessage(err);
    errorDetails = toErrorDetails(err);
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
  }

  function selectAll() {
    selected = new SvelteSet(devices.map((d) => d.mac));
  }

  function selectNone() {
    selected.clear();
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

  // reasonBadgeClass / reasonBadgeText moved with the precheck UI to
  // provision/IPListPanel.svelte (M2 — Block 4b.3).

  function selectOnlyEligible() {
    if (!precheckTemplate || precheckTemplateError) return;
    const skippedIPs = new Set(precheckIssues.map((issue) => issue.ip));
    selected = new SvelteSet(
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

  // Reactive deps must appear in the expression Svelte tracks, not behind a
  // function call. Touching each piece of state buildTemplate() reads makes
  // the statement re-run when any of them change.
  $: precheckTemplate =
    (viewMode,
    jsonText,
    sysState,
    mqttState,
    wsState,
    bleState,
    matterState,
    autoUpdateState,
    cloudState,
    authState,
    wifiState,
    wifiAPState,
    ethState,
    modbusState,
    zigbeeState,
    uiState,
    scriptsState,
    webhooksState,
    coverState,
    zigbeeOpsState,
    templateForPrecheck());
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
    // Local dedup-only Set; not reactive state — the IIFE returns a plain
    // object whose `credentialRefs` is the spread of this set's entries.
    // eslint-disable-next-line svelte/prefer-svelte-reactivity
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
  // The two `autoSelectedCredentialRef = …` writes look dead to the
  // intra-block analyser but are read on the NEXT reactive run (the
  // `=== autoSelectedCredentialRef` guards above) to decide whether the
  // user has overridden the auto-pick. ESLint can't see that across
  // reactive-block invocations, so disable on each write.
  $: if (resolvedGroupCredentialRef) {
    if (
      !selectedTemplateCredentialRef ||
      selectedTemplateCredentialRef === autoSelectedCredentialRef
    ) {
      selectedTemplateCredentialRef = resolvedGroupCredentialRef;
      // eslint-disable-next-line no-useless-assignment
      autoSelectedCredentialRef = resolvedGroupCredentialRef;
    }
  } else if (
    autoSelectedCredentialRef &&
    selectedTemplateCredentialRef === autoSelectedCredentialRef
  ) {
    selectedTemplateCredentialRef = '';
    // eslint-disable-next-line no-useless-assignment
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
    autoUpdateState = createAutoUpdateState();
    cloudState = createCloudState();
    authState = createAuthState();
    wifiState = createWifiState();
    wifiAPState = createWifiAPState();
    ethState = createEthState();
    modbusState = createModbusState();
    zigbeeState = createZigbeeState();
    uiState = createUIState();
    scriptsState = createScriptsState();
    webhooksState = createWebhooksState();
    coverState = createCoverState();
    zigbeeOpsState = createZigbeeOpsState();
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
    let nextAuth: AuthState | null = null;
    let nextWifi: WifiState | null = null;
    let nextWifiAP: WifiAPState | null = null;
    let nextEth: EthState | null = null;
    let nextModbus: ModbusState | null = null;
    let nextZigbee: ZigbeeState | null = null;
    let nextUI: UIState | null = null;
    let nextScripts: ScriptsState | null = null;
    let nextWebhooks: WebhooksState | null = null;
    let nextCover: CoverState | null = null;
    let nextAutoUpdate: AutoUpdateState | null = null;
    for (const [sectionName, rawSection] of Object.entries(template)) {
      const section = sectionName.trim().toLowerCase();
      // auto_update is a special-case section: the canonical encoding is a
      // bare string ("off"|"stable"|"beta"). Handle it before the
      // asRecord-must-be-object guard rejects non-objects.
      if (section === 'auto_update') {
        const r = hydrateAutoUpdate(rawSection);
        if (!r.ok) return r;
        nextAutoUpdate = r.state;
        continue;
      }
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
        case 'webhooks': {
          const r = hydrateWebhooks(record);
          if (!r.ok) return r;
          nextWebhooks = r.state;
          break;
        }
        case 'cover': {
          const r = hydrateCover(record);
          if (!r.ok) return r;
          nextCover = r.state;
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
    if (nextAutoUpdate) autoUpdateState = nextAutoUpdate;
    if (nextCloud) cloudState = nextCloud;
    if (nextAuth) authState = nextAuth;
    if (nextWifi) wifiState = nextWifi;
    if (nextWifiAP) wifiAPState = nextWifiAP;
    if (nextEth) ethState = nextEth;
    if (nextModbus) modbusState = nextModbus;
    if (nextZigbee) zigbeeState = nextZigbee;
    if (nextUI) uiState = nextUI;
    if (nextScripts) scriptsState = nextScripts;
    if (nextWebhooks) webhooksState = nextWebhooks;
    if (nextCover) coverState = nextCover;
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
    const autoUpdate = buildAutoUpdate(autoUpdateState);
    if (autoUpdate) out.auto_update = autoUpdate;
    const cloud = buildCloud(cloudState);
    if (cloud) out.cloud = cloud;
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
    const webhooks = buildWebhooks(webhooksState);
    if (webhooks) out.webhooks = webhooks;
    const cover = buildCover(coverState);
    if (cover) out.cover = cover;
    const zigbeeOps = buildZigbeeOps(zigbeeOpsState);
    if (zigbeeOps) {
      const existing = (out.gen2_rpc as Record<string, unknown> | undefined) ?? {};
      out.gen2_rpc = { ...existing, ...zigbeeOps };
    }
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
    <IPListPanel
      {devices}
      {selected}
      {loading}
      {precheckEligibleCount}
      {precheckIssues}
      {precheckReasonCounts}
      {precheckTemplateError}
      {copiedSkipped}
      onToggle={toggle}
      onSelectAll={selectAll}
      onSelectNone={selectNone}
      onSelectOnlyEligible={selectOnlyEligible}
      onCopySkippedIPs={copySkippedIPs}
    />
  </div>

  <div class="col-lg-6 provision-settings-col">
    <div class="card bg-dark border-secondary">
      <TemplatesPanel
        bind:selectedTemplate
        {templateOptions}
        bind:templateName
        bind:selectedTemplateCredentialRef
        {credentialOptions}
        {advancedModeEnabled}
        {viewMode}
        {groupCredentialHint}
        onLoad={loadCurrentTemplate}
        onDelete={deleteCurrentTemplate}
        onSave={saveCurrentTemplate}
        onRename={renameCurrentTemplate}
        onSetView={setView}
      />

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
              bind:autoUpdate={autoUpdateState}
              bind:cloud={cloudState}
              bind:auth={authState}
              bind:wifi={wifiState}
              bind:ui={uiState}
            />
            <WifiAPForm bind:state={wifiAPState} />
            <EthForm bind:state={ethState} />
            <ModbusForm bind:state={modbusState} />
            <ZigbeeForm bind:state={zigbeeState} />
            <ScriptsForm bind:state={scriptsState} />
            <WebhooksForm bind:state={webhooksState} />
            <CoverForm bind:state={coverState} />
            <ZigbeeOpsForm bind:state={zigbeeOpsState} />
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
  <ResultsPanel {results} {devices} {running} onError={captureError} />
{/if}
