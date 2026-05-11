<!--
  Big compliance-rules form: every SectionCard (sys / mqtt / cloud / ws /
  ble / wifi / wifi-ap / eth / matter / modbus / zigbee / auto-update /
  fw 2.0+ / custom rules) with its per-field enable toggles + the
  initToggles ↔ applyTogglesToSettings round-trip + the Save button.

  Extracted from Compliance.svelte in v0.3.0 (M2 part 5 — Block 4b.3 of
  docs/plans/phase-4b-refactor-block.md). The parent (Compliance.svelte)
  keeps the load() lifecycle + error capture; this component owns every
  toggle let, the per-section open state, the ensure/init/apply
  pipelines, the addRule / removeRule mutations, and the Save button.

  bind:settings flows mutations back to the parent so DeviceMatrix
  re-renders against the same row that just got persisted.
-->
<script lang="ts">
  import SectionCard from '../../components/SectionCard.svelte';
  import FieldRow from '../../components/FieldRow.svelte';
  import Toggle from '../../components/Toggle.svelte';
  import Select from '../../components/Select.svelte';
  import CustomRulesList from './CustomRulesList.svelte';
  import type { AppSettings, CustomRule } from '../../lib/types';

  export let settings: AppSettings;
  export let loading: boolean;
  /** Called once applyTogglesToSettings has flushed the toggle state
   * back into `settings`. The parent persists + reloads; on throw, the
   * "Saved" badge stays empty so no false success surfaces. */
  export let onSave: () => Promise<void>;

  const sourceOptions: Array<{ value: CustomRule['source']; label: string }> = [
    { value: 'device', label: 'device' },
    { value: 'config', label: 'config' },
    { value: 'status', label: 'status' },
  ];
  const opOptions: Array<{ value: CustomRule['op']; label: string }> = [
    { value: 'eq', label: 'equals' },
    { value: 'ne', label: 'not equals' },
    { value: 'contains', label: 'contains' },
    { value: 'regex', label: 'regex' },
    { value: 'exists', label: 'exists' },
  ];
  type TlsMode = NonNullable<AppSettings['compliance']['ws_tls_mode']>;
  type Ipv4Mode = NonNullable<AppSettings['compliance']['eth_ipv4mode']>;
  const tlsModeOptions: Array<{ value: TlsMode; label: string }> = [
    { value: 'no_validation', label: 'TLS — no validation' },
    { value: 'default', label: 'TLS — default' },
    { value: 'user', label: 'TLS — user CA' },
  ];
  const ipv4ModeOptions: Array<{ value: Ipv4Mode; label: string }> = [
    { value: '', label: '(any)' },
    { value: 'dhcp', label: 'DHCP' },
    { value: 'static', label: 'Static' },
  ];

  let wifiSSIDEnabled = false;

  let mqttEnabledField = false;
  let mqttServerEnabled = false;
  let mqttClientIDEnabled = false;
  let mqttTopicPrefixEnabled = false;
  let mqttRPCNtfEnabled = false;
  let mqttStatusNtfEnabled = false;
  let mqttEnableRPCEnabled = false;
  let mqttEnableControlEnabled = false;

  let cloudConnectedEnabled = false;

  let wsEnabledField = false;
  let wsConnectedField = false;
  let wsServerField = false;
  let wsTLSModeField = false;
  let wsSSLCAField = false;

  let bleGWEnabledField = false;
  let bleRPCEnabledField = false;
  let bleObserverEnabledField = false;

  let tzEnabled = false;
  let sntpEnabled = false;
  let sysDebugWSField = false;
  let sysDebugUDPHostField = false;
  let sysRPCUDPPortField = false;
  let latEnabled = false;
  let lonEnabled = false;
  let ecoEnabled = false;
  let discoverableEnabled = false;

  let wifiAPEnabledField = false;
  let wifiAPIsOpenField = false;
  let ethEnabledField = false;
  let ethIPv4ModeField = false;

  let sysDebugMQTTField = false;
  let matterEnabledField = false;
  let modbusEnabledField = false;
  let zigbeeEnabledField = false;

  // Firmware 2.0.0-beta1 compliance fields:
  let enhancedSecurityField = false;
  let tlsCertValidField = false;
  let wifiHostnameEnabled = false;
  let blePairedField = false;
  let webhooksConfiguredField = false;

  let wifiOpen = false;
  let wifiAPOpen = false;
  let ethOpen = false;
  let mqttOpen = false;
  let cloudOpen = false;
  let wsOpen = false;
  let bleOpen = false;
  let sysOpen = false;
  let matterOpen = false;
  let modbusOpen = false;
  let zigbeeOpen = false;
  let autoUpdateOpen = false;
  let fw2Open = false;

  let customOpen = false;

  $: wifiExpanded = wifiSSIDEnabled;
  $: mqttExpanded =
    mqttEnabledField ||
    mqttServerEnabled ||
    mqttClientIDEnabled ||
    mqttTopicPrefixEnabled ||
    mqttRPCNtfEnabled ||
    mqttStatusNtfEnabled ||
    mqttEnableRPCEnabled ||
    mqttEnableControlEnabled;
  $: cloudExpanded = cloudConnectedEnabled;
  $: wsExpanded =
    wsEnabledField || wsConnectedField || wsServerField || wsTLSModeField || wsSSLCAField;
  $: bleExpanded = bleGWEnabledField || bleRPCEnabledField || bleObserverEnabledField;
  $: sysExpanded =
    tzEnabled ||
    sntpEnabled ||
    sysDebugWSField ||
    sysDebugMQTTField ||
    sysDebugUDPHostField ||
    sysRPCUDPPortField ||
    latEnabled ||
    lonEnabled ||
    ecoEnabled ||
    discoverableEnabled;
  $: matterExpanded = matterEnabledField;
  $: modbusExpanded = modbusEnabledField;
  $: zigbeeExpanded = zigbeeEnabledField;
  $: wifiAPExpanded = wifiAPEnabledField || wifiAPIsOpenField;
  $: ethExpanded = ethEnabledField || ethIPv4ModeField;
  $: fw2Expanded =
    enhancedSecurityField ||
    tlsCertValidField ||
    wifiHostnameEnabled ||
    blePairedField ||
    webhooksConfiguredField;
  $: autoUpdateExpanded =
    settings.compliance.auto_update_stage !== '' &&
    settings.compliance.auto_update_stage !== undefined;
  $: customExpanded = (settings.compliance.custom_rules || []).length > 0;

  function ensureDefaults() {
    settings.compliance = settings.compliance || {};
    settings.compliance.custom_rules = settings.compliance.custom_rules || [];
    if (settings.compliance.mqtt_enabled === undefined) settings.compliance.mqtt_enabled = null;
    if (settings.compliance.cloud_connected === undefined)
      settings.compliance.cloud_connected = null;
    if (settings.compliance.ws_enabled === undefined) settings.compliance.ws_enabled = null;
    if (settings.compliance.ws_connected === undefined) settings.compliance.ws_connected = null;
    if (settings.compliance.ws_tls_mode === undefined) settings.compliance.ws_tls_mode = '';
    if (settings.compliance.ws_ssl_ca === undefined) settings.compliance.ws_ssl_ca = '';
    if (settings.compliance.ble_gw_enabled === undefined) settings.compliance.ble_gw_enabled = null;
    if (settings.compliance.ble_rpc_enable === undefined) settings.compliance.ble_rpc_enable = null;
    if (settings.compliance.ble_observer_enable === undefined)
      settings.compliance.ble_observer_enable = null;
    if (settings.compliance.mqtt_rpc_ntf === undefined) settings.compliance.mqtt_rpc_ntf = null;
    if (settings.compliance.mqtt_status_ntf === undefined)
      settings.compliance.mqtt_status_ntf = null;
    if (settings.compliance.mqtt_enable_rpc === undefined)
      settings.compliance.mqtt_enable_rpc = null;
    if (settings.compliance.mqtt_enable_control === undefined)
      settings.compliance.mqtt_enable_control = null;
    if (settings.compliance.sys_debug_websocket === undefined)
      settings.compliance.sys_debug_websocket = null;
    if (settings.compliance.sys_debug_udp_host === undefined)
      settings.compliance.sys_debug_udp_host = '';
    if (settings.compliance.sys_rpc_udp_port === undefined)
      settings.compliance.sys_rpc_udp_port = null;
    if (settings.compliance.eco_mode === undefined) settings.compliance.eco_mode = null;
    if (settings.compliance.discoverable === undefined) settings.compliance.discoverable = null;
    if (settings.compliance.wifi_ap_enabled === undefined)
      settings.compliance.wifi_ap_enabled = null;
    if (settings.compliance.wifi_ap_is_open === undefined)
      settings.compliance.wifi_ap_is_open = null;
    if (settings.compliance.eth_enabled === undefined) settings.compliance.eth_enabled = null;
    if (settings.compliance.eth_ipv4mode === undefined) settings.compliance.eth_ipv4mode = '';
    if (settings.compliance.sys_debug_mqtt === undefined) settings.compliance.sys_debug_mqtt = null;
    if (settings.compliance.matter_enabled === undefined) settings.compliance.matter_enabled = null;
    if (settings.compliance.modbus_enabled === undefined) settings.compliance.modbus_enabled = null;
    if (settings.compliance.zigbee_enabled === undefined) settings.compliance.zigbee_enabled = null;
    // Firmware 2.0.0-beta1 fields:
    if (settings.compliance.enhanced_security === undefined)
      settings.compliance.enhanced_security = null;
    if (settings.compliance.tls_cert_valid === undefined) settings.compliance.tls_cert_valid = null;
    if (settings.compliance.wifi_hostname === undefined) settings.compliance.wifi_hostname = '';
    if (settings.compliance.ble_paired === undefined) settings.compliance.ble_paired = null;
    if (settings.compliance.webhooks_configured === undefined)
      settings.compliance.webhooks_configured = null;
    if (settings.compliance.auto_update_stage === undefined)
      settings.compliance.auto_update_stage = '';
  }

  function initToggles() {
    ensureDefaults();
    wifiSSIDEnabled = Boolean(settings.compliance.wifi_ssid);

    mqttEnabledField =
      settings.compliance.mqtt_enabled !== null && settings.compliance.mqtt_enabled !== undefined;
    mqttServerEnabled = Boolean(settings.compliance.mqtt_server);
    mqttClientIDEnabled = Boolean(settings.compliance.mqtt_client_id);
    mqttTopicPrefixEnabled = Boolean(settings.compliance.mqtt_topic_prefix);
    mqttRPCNtfEnabled =
      settings.compliance.mqtt_rpc_ntf !== null && settings.compliance.mqtt_rpc_ntf !== undefined;
    mqttStatusNtfEnabled =
      settings.compliance.mqtt_status_ntf !== null &&
      settings.compliance.mqtt_status_ntf !== undefined;
    mqttEnableRPCEnabled =
      settings.compliance.mqtt_enable_rpc !== null &&
      settings.compliance.mqtt_enable_rpc !== undefined;
    mqttEnableControlEnabled =
      settings.compliance.mqtt_enable_control !== null &&
      settings.compliance.mqtt_enable_control !== undefined;

    cloudConnectedEnabled =
      settings.compliance.cloud_connected !== null &&
      settings.compliance.cloud_connected !== undefined;

    wsEnabledField =
      settings.compliance.ws_enabled !== null && settings.compliance.ws_enabled !== undefined;
    wsConnectedField =
      settings.compliance.ws_connected !== null && settings.compliance.ws_connected !== undefined;
    wsServerField = Boolean(settings.compliance.ws_server);
    wsTLSModeField = Boolean(settings.compliance.ws_tls_mode);
    wsSSLCAField = Boolean(settings.compliance.ws_ssl_ca);

    bleGWEnabledField =
      settings.compliance.ble_gw_enabled !== null &&
      settings.compliance.ble_gw_enabled !== undefined;
    bleRPCEnabledField =
      settings.compliance.ble_rpc_enable !== null &&
      settings.compliance.ble_rpc_enable !== undefined;
    bleObserverEnabledField =
      settings.compliance.ble_observer_enable !== null &&
      settings.compliance.ble_observer_enable !== undefined;

    tzEnabled = Boolean(settings.compliance.tz);
    sntpEnabled = Boolean(settings.compliance.sntp_server);
    sysDebugWSField =
      settings.compliance.sys_debug_websocket !== null &&
      settings.compliance.sys_debug_websocket !== undefined;
    sysDebugUDPHostField = Boolean(settings.compliance.sys_debug_udp_host);
    sysRPCUDPPortField =
      settings.compliance.sys_rpc_udp_port !== null &&
      settings.compliance.sys_rpc_udp_port !== undefined;
    latEnabled = settings.compliance.lat !== null && settings.compliance.lat !== undefined;
    lonEnabled = settings.compliance.lon !== null && settings.compliance.lon !== undefined;
    ecoEnabled =
      settings.compliance.eco_mode !== null && settings.compliance.eco_mode !== undefined;
    discoverableEnabled =
      settings.compliance.discoverable !== null && settings.compliance.discoverable !== undefined;

    wifiAPEnabledField =
      settings.compliance.wifi_ap_enabled !== null &&
      settings.compliance.wifi_ap_enabled !== undefined;
    wifiAPIsOpenField =
      settings.compliance.wifi_ap_is_open !== null &&
      settings.compliance.wifi_ap_is_open !== undefined;
    ethEnabledField =
      settings.compliance.eth_enabled !== null && settings.compliance.eth_enabled !== undefined;
    ethIPv4ModeField = Boolean(settings.compliance.eth_ipv4mode);

    sysDebugMQTTField =
      settings.compliance.sys_debug_mqtt !== null &&
      settings.compliance.sys_debug_mqtt !== undefined;
    matterEnabledField =
      settings.compliance.matter_enabled !== null &&
      settings.compliance.matter_enabled !== undefined;
    modbusEnabledField =
      settings.compliance.modbus_enabled !== null &&
      settings.compliance.modbus_enabled !== undefined;
    zigbeeEnabledField =
      settings.compliance.zigbee_enabled !== null &&
      settings.compliance.zigbee_enabled !== undefined;

    // Firmware 2.0.0-beta1 fields:
    enhancedSecurityField =
      settings.compliance.enhanced_security !== null &&
      settings.compliance.enhanced_security !== undefined;
    tlsCertValidField =
      settings.compliance.tls_cert_valid !== null &&
      settings.compliance.tls_cert_valid !== undefined;
    wifiHostnameEnabled = Boolean(settings.compliance.wifi_hostname);
    blePairedField =
      settings.compliance.ble_paired !== null && settings.compliance.ble_paired !== undefined;
    webhooksConfiguredField =
      settings.compliance.webhooks_configured !== null &&
      settings.compliance.webhooks_configured !== undefined;
  }

  function applyTogglesToSettings() {
    ensureDefaults();
    settings.compliance.wifi_ssid = wifiSSIDEnabled ? settings.compliance.wifi_ssid || '' : '';

    settings.compliance.mqtt_enabled = mqttEnabledField
      ? Boolean(settings.compliance.mqtt_enabled)
      : null;
    settings.compliance.mqtt_server = mqttServerEnabled
      ? settings.compliance.mqtt_server || ''
      : '';
    settings.compliance.mqtt_client_id = mqttClientIDEnabled
      ? settings.compliance.mqtt_client_id || ''
      : '';
    settings.compliance.mqtt_topic_prefix = mqttTopicPrefixEnabled
      ? settings.compliance.mqtt_topic_prefix || ''
      : '';
    settings.compliance.mqtt_rpc_ntf = mqttRPCNtfEnabled
      ? Boolean(settings.compliance.mqtt_rpc_ntf)
      : null;
    settings.compliance.mqtt_status_ntf = mqttStatusNtfEnabled
      ? Boolean(settings.compliance.mqtt_status_ntf)
      : null;
    settings.compliance.mqtt_enable_rpc = mqttEnableRPCEnabled
      ? Boolean(settings.compliance.mqtt_enable_rpc)
      : null;
    settings.compliance.mqtt_enable_control = mqttEnableControlEnabled
      ? Boolean(settings.compliance.mqtt_enable_control)
      : null;

    settings.compliance.cloud_connected = cloudConnectedEnabled
      ? Boolean(settings.compliance.cloud_connected)
      : null;

    settings.compliance.ws_enabled = wsEnabledField
      ? Boolean(settings.compliance.ws_enabled)
      : null;
    settings.compliance.ws_connected = wsConnectedField
      ? Boolean(settings.compliance.ws_connected)
      : null;
    settings.compliance.ws_server = wsServerField ? settings.compliance.ws_server || '' : '';
    settings.compliance.ws_tls_mode = wsTLSModeField
      ? settings.compliance.ws_tls_mode || 'default'
      : '';
    settings.compliance.ws_ssl_ca =
      wsSSLCAField && settings.compliance.ws_tls_mode === 'user'
        ? settings.compliance.ws_ssl_ca || ''
        : '';

    settings.compliance.ble_gw_enabled = bleGWEnabledField
      ? Boolean(settings.compliance.ble_gw_enabled)
      : null;
    settings.compliance.ble_rpc_enable = bleRPCEnabledField
      ? Boolean(settings.compliance.ble_rpc_enable)
      : null;
    settings.compliance.ble_observer_enable = bleObserverEnabledField
      ? Boolean(settings.compliance.ble_observer_enable)
      : null;

    settings.compliance.tz = tzEnabled ? settings.compliance.tz || '' : '';
    settings.compliance.sntp_server = sntpEnabled ? settings.compliance.sntp_server || '' : '';
    settings.compliance.sys_debug_websocket = sysDebugWSField
      ? Boolean(settings.compliance.sys_debug_websocket)
      : null;
    settings.compliance.sys_debug_udp_host = sysDebugUDPHostField
      ? settings.compliance.sys_debug_udp_host || ''
      : '';
    settings.compliance.sys_rpc_udp_port = sysRPCUDPPortField
      ? Number(settings.compliance.sys_rpc_udp_port ?? 0)
      : null;
    settings.compliance.lat = latEnabled ? settings.compliance.lat : null;
    settings.compliance.lon = lonEnabled ? settings.compliance.lon : null;
    settings.compliance.eco_mode = ecoEnabled ? Boolean(settings.compliance.eco_mode) : null;
    settings.compliance.discoverable = discoverableEnabled
      ? Boolean(settings.compliance.discoverable)
      : null;

    settings.compliance.wifi_ap_enabled = wifiAPEnabledField
      ? Boolean(settings.compliance.wifi_ap_enabled)
      : null;
    settings.compliance.wifi_ap_is_open = wifiAPIsOpenField
      ? Boolean(settings.compliance.wifi_ap_is_open)
      : null;
    settings.compliance.eth_enabled = ethEnabledField
      ? Boolean(settings.compliance.eth_enabled)
      : null;
    settings.compliance.eth_ipv4mode = ethIPv4ModeField
      ? settings.compliance.eth_ipv4mode || 'dhcp'
      : '';

    settings.compliance.sys_debug_mqtt = sysDebugMQTTField
      ? Boolean(settings.compliance.sys_debug_mqtt)
      : null;
    settings.compliance.matter_enabled = matterEnabledField
      ? Boolean(settings.compliance.matter_enabled)
      : null;
    settings.compliance.modbus_enabled = modbusEnabledField
      ? Boolean(settings.compliance.modbus_enabled)
      : null;
    settings.compliance.zigbee_enabled = zigbeeEnabledField
      ? Boolean(settings.compliance.zigbee_enabled)
      : null;

    settings.compliance.enhanced_security = enhancedSecurityField
      ? Boolean(settings.compliance.enhanced_security)
      : null;
    settings.compliance.tls_cert_valid = tlsCertValidField
      ? Boolean(settings.compliance.tls_cert_valid)
      : null;
    settings.compliance.wifi_hostname = wifiHostnameEnabled
      ? settings.compliance.wifi_hostname || ''
      : '';
    settings.compliance.ble_paired = blePairedField
      ? Boolean(settings.compliance.ble_paired)
      : null;
    settings.compliance.webhooks_configured = webhooksConfiguredField
      ? Boolean(settings.compliance.webhooks_configured)
      : null;
  }

  // load() + the original save() moved to the parent (Compliance.svelte);
  // the new save() below calls onSave (parent-provided) after flushing
  // toggle state via applyTogglesToSettings.

  function addRule() {
    ensureDefaults();
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
    ];
  }

  function removeRule(index: number) {
    settings.compliance.custom_rules = (settings.compliance.custom_rules || []).filter(
      (_, i) => i !== index,
    );
  }

  let saved = '';

  async function save() {
    saved = '';
    try {
      applyTogglesToSettings();
      settings.compliance.custom_rules = (settings.compliance.custom_rules || []).filter(
        (rule) => rule.path.trim() !== '',
      );
      await onSave();
      saved = 'Saved';
      setTimeout(() => {
        if (saved === 'Saved') saved = '';
      }, 1500);
    } catch {
      // Parent already captured + surfaced the error via ErrorNotice.
    }
  }

  // Re-derive toggles whenever the parent feeds us a fresh settings row
  // (e.g. after load() reloads on Save).
  $: if (settings.compliance) initToggles();
</script>

<div class="card bg-dark border-secondary">
  <div class="card-body">
    <h2 class="h5">Compliance Rules</h2>
    <p class="text-secondary mb-3">
      Section headers expand fields. Each field has a checkbox that opts it into the compliance
      check.
    </p>

    <div class="d-flex flex-column gap-3">
      <SectionCard tag="sys" bind:open={sysOpen} forceOpen={sysExpanded}>
        <div class="sa-form-grid">
          <div data-span="4">
            <FieldRow label="Timezone" bind:enabled={tzEnabled}>
              <input
                class="form-control"
                bind:value={settings.compliance.tz}
                disabled={!tzEnabled}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="SNTP Server" bind:enabled={sntpEnabled}>
              <input
                class="form-control"
                bind:value={settings.compliance.sntp_server}
                disabled={!sntpEnabled}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="Debug WebSocket" bind:enabled={sysDebugWSField}>
              <Toggle
                bind:checked={settings.compliance.sys_debug_websocket}
                disabled={!sysDebugWSField}
                label={settings.compliance.sys_debug_websocket ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="Debug MQTT" bind:enabled={sysDebugMQTTField}>
              <Toggle
                bind:checked={settings.compliance.sys_debug_mqtt}
                disabled={!sysDebugMQTTField}
                label={settings.compliance.sys_debug_mqtt ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="Debug UDP Host" bind:enabled={sysDebugUDPHostField}>
              <input
                class="form-control"
                placeholder="host:port"
                bind:value={settings.compliance.sys_debug_udp_host}
                disabled={!sysDebugUDPHostField}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="RPC UDP Port" bind:enabled={sysRPCUDPPortField}>
              <input
                class="form-control"
                type="number"
                min="0"
                bind:value={settings.compliance.sys_rpc_udp_port}
                disabled={!sysRPCUDPPortField}
              />
            </FieldRow>
          </div>
          <div data-span="3">
            <FieldRow label="Latitude" bind:enabled={latEnabled}>
              <input
                class="form-control"
                type="number"
                step="0.0001"
                bind:value={settings.compliance.lat}
                disabled={!latEnabled}
              />
            </FieldRow>
          </div>
          <div data-span="3">
            <FieldRow label="Longitude" bind:enabled={lonEnabled}>
              <input
                class="form-control"
                type="number"
                step="0.0001"
                bind:value={settings.compliance.lon}
                disabled={!lonEnabled}
              />
            </FieldRow>
          </div>
          <div data-span="3">
            <FieldRow label="Eco Mode" bind:enabled={ecoEnabled}>
              <Toggle
                bind:checked={settings.compliance.eco_mode}
                disabled={!ecoEnabled}
                label={settings.compliance.eco_mode ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
          <div data-span="3">
            <FieldRow label="Discoverable" bind:enabled={discoverableEnabled}>
              <Toggle
                bind:checked={settings.compliance.discoverable}
                disabled={!discoverableEnabled}
                label={settings.compliance.discoverable ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
        </div>
      </SectionCard>

      <SectionCard tag="mqtt" bind:open={mqttOpen} forceOpen={mqttExpanded}>
        <div class="sa-form-grid">
          <div data-span="4">
            <FieldRow label="Enabled" bind:enabled={mqttEnabledField}>
              <Toggle
                bind:checked={settings.compliance.mqtt_enabled}
                disabled={!mqttEnabledField}
                label={settings.compliance.mqtt_enabled ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="Broker" bind:enabled={mqttServerEnabled}>
              <input
                class="form-control"
                bind:value={settings.compliance.mqtt_server}
                disabled={!mqttServerEnabled}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="Client ID" bind:enabled={mqttClientIDEnabled}>
              <input
                class="form-control"
                bind:value={settings.compliance.mqtt_client_id}
                disabled={!mqttClientIDEnabled}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="Topic Prefix" bind:enabled={mqttTopicPrefixEnabled}>
              <input
                class="form-control"
                bind:value={settings.compliance.mqtt_topic_prefix}
                disabled={!mqttTopicPrefixEnabled}
              />
            </FieldRow>
          </div>
          <div data-span="2">
            <FieldRow label="rpc_ntf" bind:enabled={mqttRPCNtfEnabled}>
              <Toggle
                bind:checked={settings.compliance.mqtt_rpc_ntf}
                disabled={!mqttRPCNtfEnabled}
                label={settings.compliance.mqtt_rpc_ntf ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
          <div data-span="2">
            <FieldRow label="status_ntf" bind:enabled={mqttStatusNtfEnabled}>
              <Toggle
                bind:checked={settings.compliance.mqtt_status_ntf}
                disabled={!mqttStatusNtfEnabled}
                label={settings.compliance.mqtt_status_ntf ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
          <div data-span="2">
            <FieldRow label="enable_rpc" bind:enabled={mqttEnableRPCEnabled}>
              <Toggle
                bind:checked={settings.compliance.mqtt_enable_rpc}
                disabled={!mqttEnableRPCEnabled}
                label={settings.compliance.mqtt_enable_rpc ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
          <div data-span="2">
            <FieldRow label="enable_control" bind:enabled={mqttEnableControlEnabled}>
              <Toggle
                bind:checked={settings.compliance.mqtt_enable_control}
                disabled={!mqttEnableControlEnabled}
                label={settings.compliance.mqtt_enable_control ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
        </div>
      </SectionCard>

      <SectionCard tag="cloud" bind:open={cloudOpen} forceOpen={cloudExpanded}>
        <div class="sa-form-grid">
          <div data-span="4">
            <FieldRow label="Connected" bind:enabled={cloudConnectedEnabled}>
              <Toggle
                bind:checked={settings.compliance.cloud_connected}
                disabled={!cloudConnectedEnabled}
                label={settings.compliance.cloud_connected ? 'Yes' : 'No'}
              />
            </FieldRow>
          </div>
        </div>
      </SectionCard>

      <SectionCard tag="ws" bind:open={wsOpen} forceOpen={wsExpanded}>
        <div class="sa-form-grid">
          <div data-span="3">
            <FieldRow label="Enabled" bind:enabled={wsEnabledField}>
              <Toggle
                bind:checked={settings.compliance.ws_enabled}
                disabled={!wsEnabledField}
                label={settings.compliance.ws_enabled ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
          <div data-span="3">
            <FieldRow label="Connected" bind:enabled={wsConnectedField}>
              <Toggle
                bind:checked={settings.compliance.ws_connected}
                disabled={!wsConnectedField}
                label={settings.compliance.ws_connected ? 'Yes' : 'No'}
              />
            </FieldRow>
          </div>
          <div data-span="3">
            <FieldRow label="Server" bind:enabled={wsServerField}>
              <input
                class="form-control"
                bind:value={settings.compliance.ws_server}
                disabled={!wsServerField}
              />
            </FieldRow>
          </div>
          <div data-span="3">
            <FieldRow label="Connection type" bind:enabled={wsTLSModeField}>
              <Select
                bind:value={settings.compliance.ws_tls_mode}
                options={tlsModeOptions}
                disabled={!wsTLSModeField}
                ariaLabel="Connection type"
              />
            </FieldRow>
          </div>
          <div data-span="3">
            <FieldRow label="TLS / SSL CA" bind:enabled={wsSSLCAField}>
              <input
                class="form-control"
                placeholder="* or ca.pem"
                bind:value={settings.compliance.ws_ssl_ca}
                disabled={!wsSSLCAField || settings.compliance.ws_tls_mode !== 'user'}
              />
            </FieldRow>
          </div>
        </div>
      </SectionCard>

      <SectionCard tag="ble" bind:open={bleOpen} forceOpen={bleExpanded}>
        <div class="sa-form-grid">
          <div data-span="4">
            <FieldRow label="Gateway Enabled" bind:enabled={bleGWEnabledField}>
              <Toggle
                bind:checked={settings.compliance.ble_gw_enabled}
                disabled={!bleGWEnabledField}
                label={settings.compliance.ble_gw_enabled ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="RPC over BLE" bind:enabled={bleRPCEnabledField}>
              <Toggle
                bind:checked={settings.compliance.ble_rpc_enable}
                disabled={!bleRPCEnabledField}
                label={settings.compliance.ble_rpc_enable ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="Observer Mode" bind:enabled={bleObserverEnabledField}>
              <Toggle
                bind:checked={settings.compliance.ble_observer_enable}
                disabled={!bleObserverEnabledField}
                label={settings.compliance.ble_observer_enable ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
        </div>
      </SectionCard>

      <SectionCard tag="wifi" bind:open={wifiOpen} forceOpen={wifiExpanded}>
        <div class="sa-form-grid">
          <div data-span="6">
            <FieldRow label="WiFi SSID" bind:enabled={wifiSSIDEnabled}>
              <input
                class="form-control"
                bind:value={settings.compliance.wifi_ssid}
                disabled={!wifiSSIDEnabled}
              />
            </FieldRow>
          </div>
        </div>
      </SectionCard>

      <SectionCard tag="wifi ap" bind:open={wifiAPOpen} forceOpen={wifiAPExpanded}>
        <div class="sa-form-grid">
          <div data-span="4">
            <FieldRow label="AP Enabled" bind:enabled={wifiAPEnabledField}>
              <Toggle
                bind:checked={settings.compliance.wifi_ap_enabled}
                disabled={!wifiAPEnabledField}
                label={settings.compliance.wifi_ap_enabled ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="Open AP" bind:enabled={wifiAPIsOpenField}>
              <Toggle
                bind:checked={settings.compliance.wifi_ap_is_open}
                disabled={!wifiAPIsOpenField}
                label={settings.compliance.wifi_ap_is_open ? 'Yes' : 'No'}
              />
            </FieldRow>
          </div>
        </div>
      </SectionCard>

      <SectionCard tag="eth" bind:open={ethOpen} forceOpen={ethExpanded}>
        <div class="sa-form-grid">
          <div data-span="4">
            <FieldRow label="Enabled" bind:enabled={ethEnabledField}>
              <Toggle
                bind:checked={settings.compliance.eth_enabled}
                disabled={!ethEnabledField}
                label={settings.compliance.eth_enabled ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="IPv4 Mode" bind:enabled={ethIPv4ModeField}>
              <Select
                bind:value={settings.compliance.eth_ipv4mode}
                options={ipv4ModeOptions}
                disabled={!ethIPv4ModeField}
                ariaLabel="IPv4 Mode"
              />
            </FieldRow>
          </div>
        </div>
      </SectionCard>

      <SectionCard tag="matter" bind:open={matterOpen} forceOpen={matterExpanded}>
        <div class="sa-form-grid">
          <div data-span="4">
            <FieldRow label="Matter Enabled" bind:enabled={matterEnabledField}>
              <Toggle
                bind:checked={settings.compliance.matter_enabled}
                disabled={!matterEnabledField}
                label={settings.compliance.matter_enabled ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
        </div>
      </SectionCard>

      <SectionCard tag="modbus" bind:open={modbusOpen} forceOpen={modbusExpanded}>
        <div class="sa-form-grid">
          <div data-span="4">
            <FieldRow label="Modbus Enabled" bind:enabled={modbusEnabledField}>
              <Toggle
                bind:checked={settings.compliance.modbus_enabled}
                disabled={!modbusEnabledField}
                label={settings.compliance.modbus_enabled ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
        </div>
      </SectionCard>

      <SectionCard tag="zigbee" bind:open={zigbeeOpen} forceOpen={zigbeeExpanded}>
        <div class="sa-form-grid">
          <div data-span="4">
            <FieldRow label="Zigbee Enabled" bind:enabled={zigbeeEnabledField}>
              <Toggle
                bind:checked={settings.compliance.zigbee_enabled}
                disabled={!zigbeeEnabledField}
                label={settings.compliance.zigbee_enabled ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
        </div>
      </SectionCard>

      <SectionCard
        tag="auto-update"
        title="Auto-Update Schedule (Gen 2+, FW 1.2.0+)"
        bind:open={autoUpdateOpen}
        forceOpen={autoUpdateExpanded}
      >
        <p class="text-secondary mb-2 text-hint-md">
          Synthesised from <code>Schedule.List</code> on each device. Skipped on devices that haven't
          been firmware-checked yet (so mixed fleets don't false-positive).
        </p>
        <div class="sa-form-grid">
          <div data-span="4">
            <label class="form-label" for="compliance-auto-update-stage">Required setting</label>
            <select
              id="compliance-auto-update-stage"
              class="form-select"
              bind:value={settings.compliance.auto_update_stage}
              title="Required value for the device's local auto-update schedule. (don't check) skips this rule."
            >
              <option value="">(don't check)</option>
              <option value="off">Off (no schedule)</option>
              <option value="stable">Stable</option>
              <option value="beta">Beta</option>
            </select>
          </div>
        </div>
      </SectionCard>

      <SectionCard
        tag="fw 2.0+"
        title="Firmware 2.0+ checks"
        bind:open={fw2Open}
        forceOpen={fw2Expanded}
      >
        <p class="text-secondary mb-2 text-hint-md">
          These rules are skipped on devices that don't report the underlying state, so mixed-fleet
          (1.x + 2.0) compliance won't false-positive.
        </p>
        <div class="sa-form-grid">
          <div data-span="4">
            <FieldRow label="Enhanced Security" bind:enabled={enhancedSecurityField}>
              <Toggle
                bind:checked={settings.compliance.enhanced_security}
                disabled={!enhancedSecurityField}
                label={settings.compliance.enhanced_security ? 'On' : 'Off'}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="TLS Certificate Valid" bind:enabled={tlsCertValidField}>
              <Toggle
                bind:checked={settings.compliance.tls_cert_valid}
                disabled={!tlsCertValidField}
                label={settings.compliance.tls_cert_valid ? 'Valid' : 'Invalid'}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="WiFi Hostname" bind:enabled={wifiHostnameEnabled}>
              <input
                class="form-control"
                placeholder={'{device_name}'}
                bind:value={settings.compliance.wifi_hostname}
                disabled={!wifiHostnameEnabled}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="BLE Paired (RPC)" bind:enabled={blePairedField}>
              <Toggle
                bind:checked={settings.compliance.ble_paired}
                disabled={!blePairedField}
                label={settings.compliance.ble_paired ? 'Required' : 'Forbidden'}
              />
            </FieldRow>
          </div>
          <div data-span="4">
            <FieldRow label="Webhooks Configured" bind:enabled={webhooksConfiguredField}>
              <Toggle
                bind:checked={settings.compliance.webhooks_configured}
                disabled={!webhooksConfiguredField}
                label={settings.compliance.webhooks_configured ? 'Required' : 'Forbidden'}
              />
            </FieldRow>
          </div>
        </div>
      </SectionCard>

      <CustomRulesList
        rules={settings.compliance.custom_rules || []}
        bind:open={customOpen}
        forceOpen={customExpanded}
        {sourceOptions}
        {opOptions}
        onAdd={addRule}
        onRemove={removeRule}
      />
    </div>

    <button class="btn btn-warning text-dark mt-3" on:click={save} disabled={loading}
      >Save Compliance</button
    >
    {#if saved}<span class="ms-2 text-success">{saved}</span>{/if}
  </div>
</div>
