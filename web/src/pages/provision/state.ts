import type {
  AuthState,
  BleState,
  CloudState,
  EthState,
  HydrateResult,
  MatterState,
  ModbusState,
  MqttState,
  OtaState,
  ScriptEntry,
  ScriptsState,
  SysState,
  UIState,
  WifiAPState,
  WifiRoamState,
  WifiStaEntry,
  WifiState,
  WsState,
  ZigbeeState,
} from './types';

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function hasOnlyKeys(record: Record<string, unknown>, keys: string[]): boolean {
  return Object.keys(record).every((key) => keys.includes(key));
}

function boolField(record: Record<string, unknown>, key: string): boolean | undefined {
  const value = record[key];
  return typeof value === 'boolean' ? value : undefined;
}

function stringField(record: Record<string, unknown>, key: string): string | undefined {
  const value = record[key];
  return typeof value === 'string' ? value : undefined;
}

function numberField(record: Record<string, unknown>, key: string): number | undefined {
  const value = record[key];
  return typeof value === 'number' ? value : undefined;
}

function maybeNum(raw: string | number): number | undefined {
  if (typeof raw === 'number') return Number.isFinite(raw) ? raw : undefined;
  if (raw.trim() === '') return undefined;
  const n = Number(raw);
  return Number.isFinite(n) ? n : undefined;
}

export function isTLSServerURL(raw: string): boolean {
  return raw.trim().toLowerCase().startsWith('wss://');
}

function inferWSTLSMode(
  server: string | undefined,
  sslCA: string | undefined,
  explicitMode: string | undefined,
): 'no_validation' | 'default' | 'user' | undefined {
  if (explicitMode === 'no_validation' || explicitMode === 'default' || explicitMode === 'user')
    return explicitMode;
  if (!server || !isTLSServerURL(server)) return undefined;
  if (sslCA === '*') return 'no_validation';
  if (sslCA && sslCA.trim() !== '') return 'user';
  return 'default';
}

// --- sys ---

export function createSysState(): SysState {
  return {
    enabled: false,
    nameEnabled: false,
    name: '{device_name}',
    tzEnabled: false,
    tz: 'Europe/Berlin',
    latEnabled: false,
    lat: '',
    lonEnabled: false,
    lon: '',
    sntpEnabled: false,
    sntp: 'time.cloudflare.com',
    debugWSEnabled: false,
    debugWS: false,
    debugMQTTEnabled: false,
    debugMQTT: false,
    debugUDPHostEnabled: false,
    debugUDPHost: '',
    rpcUDPPortEnabled: false,
    rpcUDPPort: '0',
    ecoEnabled: false,
    eco: false,
    discoverableEnabled: false,
    discoverable: true,
    open: false,
  };
}

export function buildSys(s: SysState): Record<string, unknown> | null {
  if (!s.enabled) return null;
  const sys: Record<string, unknown> = {};
  const deviceCfg: Record<string, unknown> = {};
  const location: Record<string, unknown> = {};
  const sntp: Record<string, unknown> = {};
  const debug: Record<string, unknown> = {};
  const debugWS: Record<string, unknown> = {};
  const debugMQTT: Record<string, unknown> = {};
  const debugUDP: Record<string, unknown> = {};
  const rpcUDP: Record<string, unknown> = {};

  if (s.nameEnabled) deviceCfg.name = s.name;
  if (s.ecoEnabled) deviceCfg.eco_mode = s.eco;
  if (s.discoverableEnabled) deviceCfg.discoverable = s.discoverable;
  if (s.tzEnabled) location.tz = s.tz;
  if (s.sntpEnabled) sntp.server = s.sntp;
  if (s.debugWSEnabled) debugWS.enable = s.debugWS;
  if (s.debugMQTTEnabled) debugMQTT.enable = s.debugMQTT;
  if (s.debugUDPHostEnabled && s.debugUDPHost.trim()) debugUDP.addr = s.debugUDPHost.trim();
  if (s.rpcUDPPortEnabled) {
    const port = maybeNum(s.rpcUDPPort);
    rpcUDP.listen_port = port === undefined ? 0 : port;
  }
  if (s.latEnabled) {
    const lat = maybeNum(s.lat);
    if (lat !== undefined) location.lat = lat;
  }
  if (s.lonEnabled) {
    const lon = maybeNum(s.lon);
    if (lon !== undefined) location.lon = lon;
  }
  if (Object.keys(deviceCfg).length > 0) sys.device = deviceCfg;
  if (Object.keys(location).length > 0) sys.location = location;
  if (Object.keys(sntp).length > 0) sys.sntp = sntp;
  if (Object.keys(debugWS).length > 0) debug.websocket = debugWS;
  if (Object.keys(debugMQTT).length > 0) debug.mqtt = debugMQTT;
  if (Object.keys(debugUDP).length > 0) debug.udp = debugUDP;
  if (Object.keys(debug).length > 0) sys.debug = debug;
  if (Object.keys(rpcUDP).length > 0) sys.rpc_udp = rpcUDP;
  return Object.keys(sys).length > 0 ? sys : null;
}

export function hydrateSys(record: Record<string, unknown>): HydrateResult<SysState> {
  if (
    !hasOnlyKeys(record, [
      'name',
      'device',
      'tz',
      'location',
      'sntp',
      'dbg',
      'debug',
      'rpc_udp',
      'lat',
      'lng',
      'lon',
      'profile',
      'addon_type',
    ])
  ) {
    return { ok: false, reason: 'Template sys section contains fields the form cannot represent.' };
  }
  const device = record.device ? asRecord(record.device) : null;
  const location = record.location ? asRecord(record.location) : null;
  const sntp = record.sntp ? asRecord(record.sntp) : null;
  const dbg = record.dbg ? asRecord(record.dbg) : null;
  const debug = record.debug ? asRecord(record.debug) : null;
  const rpcUDP = record.rpc_udp ? asRecord(record.rpc_udp) : null;
  if (
    (record.device && !device) ||
    (record.location && !location) ||
    (record.sntp && !sntp) ||
    (record.dbg && !dbg) ||
    (record.debug && !debug) ||
    (record.rpc_udp && !rpcUDP)
  ) {
    return {
      ok: false,
      reason: 'Template sys section contains nested values the form cannot represent.',
    };
  }
  if (device && !hasOnlyKeys(device, ['name', 'eco_mode', 'discoverable'])) {
    return { ok: false, reason: 'Template sys.device section contains unsupported fields.' };
  }
  if (location && !hasOnlyKeys(location, ['tz', 'lat', 'lon'])) {
    return { ok: false, reason: 'Template sys.location section contains unsupported fields.' };
  }
  if (sntp && !hasOnlyKeys(sntp, ['server'])) {
    return { ok: false, reason: 'Template sys.sntp section contains unsupported fields.' };
  }
  if (dbg && !hasOnlyKeys(dbg, ['websocket_enable', 'udp_addr'])) {
    return { ok: false, reason: 'Template sys.dbg section contains unsupported fields.' };
  }
  const debugWS = debug && debug.websocket ? asRecord(debug.websocket) : null;
  const debugMQTTRec = debug && debug.mqtt ? asRecord(debug.mqtt) : null;
  const debugUDP = debug && debug.udp ? asRecord(debug.udp) : null;
  if (
    (debug && debug.websocket && !debugWS) ||
    (debug && debug.mqtt && !debugMQTTRec) ||
    (debug && debug.udp && !debugUDP)
  ) {
    return { ok: false, reason: 'Template sys.debug section contains unsupported nested values.' };
  }
  if (debugWS && !hasOnlyKeys(debugWS, ['enable'])) {
    return {
      ok: false,
      reason: 'Template sys.debug.websocket section contains unsupported fields.',
    };
  }
  if (debugMQTTRec && !hasOnlyKeys(debugMQTTRec, ['enable'])) {
    return { ok: false, reason: 'Template sys.debug.mqtt section contains unsupported fields.' };
  }
  if (debugUDP && !hasOnlyKeys(debugUDP, ['addr'])) {
    return { ok: false, reason: 'Template sys.debug.udp section contains unsupported fields.' };
  }
  if (rpcUDP && !hasOnlyKeys(rpcUDP, ['port', 'listen_port'])) {
    return { ok: false, reason: 'Template sys.rpc_udp section contains unsupported fields.' };
  }

  const state = createSysState();
  state.enabled = true;
  const topName = stringField(record, 'name');
  const nestedName = device ? stringField(device, 'name') : undefined;
  if (topName !== undefined || nestedName !== undefined) {
    if (topName !== undefined && nestedName !== undefined && topName !== nestedName) {
      return {
        ok: false,
        reason: 'Template sys name fields disagree and cannot be represented safely in the form.',
      };
    }
    state.nameEnabled = true;
    state.name = topName ?? nestedName ?? state.name;
  }
  const topTZ = stringField(record, 'tz');
  const nestedTZ = location ? stringField(location, 'tz') : undefined;
  if (topTZ !== undefined || nestedTZ !== undefined) {
    if (topTZ !== undefined && nestedTZ !== undefined && topTZ !== nestedTZ) {
      return {
        ok: false,
        reason:
          'Template sys timezone fields disagree and cannot be represented safely in the form.',
      };
    }
    state.tzEnabled = true;
    state.tz = topTZ ?? nestedTZ ?? state.tz;
  }
  const topLat = numberField(record, 'lat');
  const nestedLat = location ? numberField(location, 'lat') : undefined;
  if (topLat !== undefined || nestedLat !== undefined) {
    if (topLat !== undefined && nestedLat !== undefined && topLat !== nestedLat) {
      return {
        ok: false,
        reason:
          'Template sys latitude fields disagree and cannot be represented safely in the form.',
      };
    }
    state.latEnabled = true;
    state.lat = String(topLat ?? nestedLat ?? '');
  }
  const topLon = numberField(record, 'lng') ?? numberField(record, 'lon');
  const nestedLon = location ? numberField(location, 'lon') : undefined;
  if (topLon !== undefined || nestedLon !== undefined) {
    if (topLon !== undefined && nestedLon !== undefined && topLon !== nestedLon) {
      return {
        ok: false,
        reason:
          'Template sys longitude fields disagree and cannot be represented safely in the form.',
      };
    }
    state.lonEnabled = true;
    state.lon = String(topLon ?? nestedLon ?? '');
  }
  const sntpServer = sntp ? stringField(sntp, 'server') : undefined;
  if (sntpServer !== undefined) {
    state.sntpEnabled = true;
    state.sntp = sntpServer;
  }
  const legacyDebugWS = dbg ? boolField(dbg, 'websocket_enable') : undefined;
  const nestedDebugWebsocket = debugWS ? boolField(debugWS, 'enable') : undefined;
  const finalDebugWS = legacyDebugWS !== undefined ? legacyDebugWS : nestedDebugWebsocket;
  if (finalDebugWS !== undefined) {
    state.debugWSEnabled = true;
    state.debugWS = finalDebugWS;
  }
  const debugMQTTValue = debugMQTTRec ? boolField(debugMQTTRec, 'enable') : undefined;
  if (debugMQTTValue !== undefined) {
    state.debugMQTTEnabled = true;
    state.debugMQTT = debugMQTTValue;
  }
  const legacyDebugUDPHost = dbg ? stringField(dbg, 'udp_addr') : undefined;
  const nestedDebugUDPHost = debugUDP ? stringField(debugUDP, 'addr') : undefined;
  const debugUDPHost = legacyDebugUDPHost ?? nestedDebugUDPHost;
  if (debugUDPHost !== undefined) {
    state.debugUDPHostEnabled = true;
    state.debugUDPHost = debugUDPHost;
  }
  const rpcUDPPort = rpcUDP
    ? (numberField(rpcUDP, 'listen_port') ?? numberField(rpcUDP, 'port'))
    : undefined;
  if (rpcUDPPort !== undefined) {
    state.rpcUDPPortEnabled = true;
    state.rpcUDPPort = String(rpcUDPPort);
  }
  const ecoMode = device ? boolField(device, 'eco_mode') : undefined;
  if (ecoMode !== undefined) {
    state.ecoEnabled = true;
    state.eco = ecoMode;
  }
  const discoverable = device ? boolField(device, 'discoverable') : undefined;
  if (discoverable !== undefined) {
    state.discoverableEnabled = true;
    state.discoverable = discoverable;
  }
  return { ok: true, state };
}

// --- mqtt ---

export function createMqttState(): MqttState {
  return {
    enabled: false,
    enableField: false,
    enable: true,
    serverEnabled: false,
    server: 'mqtt.home:1883',
    clientIDEnabled: false,
    clientID: '{device_name}',
    topicPrefixEnabled: false,
    topicPrefix: 'shelly/{device_name}',
    userEnabled: false,
    user: '',
    passEnabled: false,
    pass: '',
    sslCAEnabled: false,
    sslCA: '',
    rpcNtfEnabled: false,
    rpcNtf: true,
    statusNtfEnabled: false,
    statusNtf: true,
    enableRPCEnabled: false,
    enableRPC: true,
    enableControlEnabled: false,
    enableControl: true,
    useClientCertEnabled: false,
    useClientCert: false,
    open: false,
  };
}

export function buildMqtt(s: MqttState): Record<string, unknown> | null {
  if (!s.enabled) return null;
  const mqtt: Record<string, unknown> = {};
  if (s.enableField) mqtt.enable = s.enable;
  if (s.serverEnabled) mqtt.server = s.server;
  if (s.clientIDEnabled) {
    mqtt.client_id = s.clientID;
    mqtt.id = s.clientID;
  }
  if (s.topicPrefixEnabled) mqtt.topic_prefix = s.topicPrefix;
  if (s.userEnabled) mqtt.user = s.user;
  if (s.passEnabled) mqtt.pass = s.pass;
  if (s.sslCAEnabled && s.sslCA !== '') mqtt.ssl_ca = s.sslCA;
  if (s.rpcNtfEnabled) mqtt.rpc_ntf = s.rpcNtf;
  if (s.statusNtfEnabled) mqtt.status_ntf = s.statusNtf;
  if (s.enableRPCEnabled) mqtt.enable_rpc = s.enableRPC;
  if (s.enableControlEnabled) mqtt.enable_control = s.enableControl;
  if (s.useClientCertEnabled) mqtt.use_client_cert = s.useClientCert;
  return Object.keys(mqtt).length > 0 ? mqtt : null;
}

export function hydrateMqtt(record: Record<string, unknown>): HydrateResult<MqttState> {
  if (
    !hasOnlyKeys(record, [
      'enable',
      'server',
      'client_id',
      'id',
      'topic_prefix',
      'user',
      'pass',
      'ssl_ca',
      'rpc_ntf',
      'status_ntf',
      'enable_rpc',
      'enable_control',
      'use_client_cert',
    ])
  ) {
    return { ok: false, reason: 'Template mqtt section contains unsupported fields.' };
  }
  const clientID = stringField(record, 'client_id');
  const aliasID = stringField(record, 'id');
  if (clientID !== undefined && aliasID !== undefined && clientID !== aliasID) {
    return {
      ok: false,
      reason:
        'Template mqtt client identifiers disagree and cannot be represented safely in the form.',
    };
  }
  const state = createMqttState();
  state.enabled = true;
  const enableValue = boolField(record, 'enable');
  if (enableValue !== undefined) {
    state.enableField = true;
    state.enable = enableValue;
  }
  const serverValue = stringField(record, 'server');
  if (serverValue !== undefined) {
    state.serverEnabled = true;
    state.server = serverValue;
  }
  const clientValue = clientID ?? aliasID;
  if (clientValue !== undefined) {
    state.clientIDEnabled = true;
    state.clientID = clientValue;
  }
  const topicValue = stringField(record, 'topic_prefix');
  if (topicValue !== undefined) {
    state.topicPrefixEnabled = true;
    state.topicPrefix = topicValue;
  }
  const userValue = stringField(record, 'user');
  if (userValue !== undefined) {
    state.userEnabled = true;
    state.user = userValue;
  }
  const passValue = stringField(record, 'pass');
  if (passValue !== undefined) {
    state.passEnabled = true;
    state.pass = passValue;
  }
  const sslCAValue = stringField(record, 'ssl_ca');
  if (sslCAValue !== undefined) {
    state.sslCAEnabled = true;
    state.sslCA = sslCAValue;
  }
  const rpcValue = boolField(record, 'rpc_ntf');
  if (rpcValue !== undefined) {
    state.rpcNtfEnabled = true;
    state.rpcNtf = rpcValue;
  }
  const statusValue = boolField(record, 'status_ntf');
  if (statusValue !== undefined) {
    state.statusNtfEnabled = true;
    state.statusNtf = statusValue;
  }
  const enableRPCValue = boolField(record, 'enable_rpc');
  if (enableRPCValue !== undefined) {
    state.enableRPCEnabled = true;
    state.enableRPC = enableRPCValue;
  }
  const enableControlValue = boolField(record, 'enable_control');
  if (enableControlValue !== undefined) {
    state.enableControlEnabled = true;
    state.enableControl = enableControlValue;
  }
  const useClientCertValue = boolField(record, 'use_client_cert');
  if (useClientCertValue !== undefined) {
    state.useClientCertEnabled = true;
    state.useClientCert = useClientCertValue;
  }
  return { ok: true, state };
}

// --- ws ---

export function createWsState(): WsState {
  return {
    enabled: false,
    enableField: false,
    enable: true,
    serverEnabled: false,
    server: 'ws://ha.home:8123/api/shelly/ws',
    tlsModeEnabled: false,
    tlsMode: 'default',
    sslCAEnabled: false,
    sslCA: '',
    open: false,
  };
}

export function buildWs(s: WsState): Record<string, unknown> | null {
  if (!s.enabled) return null;
  const ws: Record<string, unknown> = {};
  if (s.enableField) ws.enable = s.enable;
  if (s.serverEnabled) ws.server = s.server;
  if (isTLSServerURL(s.server)) {
    if (s.tlsModeEnabled) ws.tls_mode = s.tlsMode;
    if (s.sslCAEnabled && s.tlsMode === 'user') ws.ssl_ca = s.sslCA;
  }
  return Object.keys(ws).length > 0 ? ws : null;
}

export function hydrateWs(record: Record<string, unknown>): HydrateResult<WsState> {
  if (!hasOnlyKeys(record, ['enable', 'server', 'tls_mode', 'ssl_ca'])) {
    return { ok: false, reason: 'Template ws section contains unsupported fields.' };
  }
  const tlsMode = stringField(record, 'tls_mode');
  if (
    tlsMode !== undefined &&
    tlsMode !== 'no_validation' &&
    tlsMode !== 'default' &&
    tlsMode !== 'user'
  ) {
    return { ok: false, reason: 'Template ws tls_mode is not representable in the form.' };
  }
  const state = createWsState();
  state.enabled = true;
  const enableValue = boolField(record, 'enable');
  if (enableValue !== undefined) {
    state.enableField = true;
    state.enable = enableValue;
  }
  const serverValue = stringField(record, 'server');
  if (serverValue !== undefined) {
    state.serverEnabled = true;
    state.server = serverValue;
  }
  const sslCAValue = stringField(record, 'ssl_ca');
  const inferredTLSMode = inferWSTLSMode(serverValue, sslCAValue, tlsMode);
  if (inferredTLSMode !== undefined) {
    state.tlsModeEnabled = true;
    state.tlsMode = inferredTLSMode;
  }
  if (sslCAValue !== undefined && sslCAValue !== '*') {
    state.sslCAEnabled = true;
    state.sslCA = sslCAValue;
  }
  return { ok: true, state };
}

// --- ble ---

export function createBleState(): BleState {
  return {
    enabled: false,
    enableField: false,
    enable: true,
    rpcEnabledField: false,
    rpcEnabled: false,
    observerEnabledField: false,
    observerEnabled: false,
    open: false,
  };
}

export function buildBle(s: BleState): Record<string, unknown> | null {
  if (!s.enabled) return null;
  const ble: Record<string, unknown> = {};
  if (s.enableField) ble.enable = s.enable;
  if (s.rpcEnabledField) ble.rpc = { enable: s.rpcEnabled };
  if (s.observerEnabledField) ble.observer = { enable: s.observerEnabled };
  return Object.keys(ble).length > 0 ? ble : null;
}

export function hydrateBle(record: Record<string, unknown>): HydrateResult<BleState> {
  if (!hasOnlyKeys(record, ['enable', 'rpc', 'observer'])) {
    return { ok: false, reason: 'Template ble section contains unsupported fields.' };
  }
  const rpc = record.rpc ? asRecord(record.rpc) : null;
  const observer = record.observer ? asRecord(record.observer) : null;
  if ((record.rpc && !rpc) || (record.observer && !observer)) {
    return {
      ok: false,
      reason: 'Template ble section contains nested values the form cannot represent.',
    };
  }
  if (rpc && !hasOnlyKeys(rpc, ['enable'])) {
    return { ok: false, reason: 'Template ble.rpc section contains unsupported fields.' };
  }
  if (observer && !hasOnlyKeys(observer, ['enable'])) {
    return { ok: false, reason: 'Template ble.observer section contains unsupported fields.' };
  }
  const state = createBleState();
  state.enabled = true;
  const enableValue = boolField(record, 'enable');
  if (enableValue !== undefined) {
    state.enableField = true;
    state.enable = enableValue;
  }
  const rpcValue = rpc ? boolField(rpc, 'enable') : undefined;
  if (rpcValue !== undefined) {
    state.rpcEnabledField = true;
    state.rpcEnabled = rpcValue;
  }
  const observerValue = observer ? boolField(observer, 'enable') : undefined;
  if (observerValue !== undefined) {
    state.observerEnabledField = true;
    state.observerEnabled = observerValue;
  }
  return { ok: true, state };
}

// --- matter ---

export function createMatterState(): MatterState {
  return { enabled: false, enableField: false, enable: true, open: false };
}

export function buildMatter(s: MatterState): Record<string, unknown> | null {
  if (!s.enabled) return null;
  const matter: Record<string, unknown> = {};
  if (s.enableField) matter.enable = s.enable;
  return Object.keys(matter).length > 0 ? matter : null;
}

export function hydrateMatter(record: Record<string, unknown>): HydrateResult<MatterState> {
  if (!hasOnlyKeys(record, ['enable'])) {
    return { ok: false, reason: 'Template matter section contains unsupported fields.' };
  }
  const state = createMatterState();
  state.enabled = true;
  const enableValue = boolField(record, 'enable');
  if (enableValue !== undefined) {
    state.enableField = true;
    state.enable = enableValue;
  }
  return { ok: true, state };
}

// --- cloud ---

export function createCloudState(): CloudState {
  return { enabled: false, enableField: false, enable: true, open: false };
}

export function buildCloud(s: CloudState): Record<string, unknown> | null {
  if (!s.enabled) return null;
  const cloud: Record<string, unknown> = {};
  if (s.enableField) cloud.enable = s.enable;
  return Object.keys(cloud).length > 0 ? cloud : null;
}

export function hydrateCloud(record: Record<string, unknown>): HydrateResult<CloudState> {
  if (!hasOnlyKeys(record, ['enable'])) {
    return { ok: false, reason: 'Template cloud section contains unsupported fields.' };
  }
  const state = createCloudState();
  state.enabled = true;
  const enableValue = boolField(record, 'enable');
  if (enableValue !== undefined) {
    state.enableField = true;
    state.enable = enableValue;
  }
  return { ok: true, state };
}

// --- ota ---

export function createOtaState(): OtaState {
  return {
    enabled: false,
    stageEnabled: false,
    stage: 'stable',
    autoUpdateEnabled: false,
    autoUpdate: 'off',
    open: false,
  };
}

export function buildOta(s: OtaState): Record<string, unknown> | null {
  if (!s.enabled) return null;
  const ota: Record<string, unknown> = {};
  if (s.stageEnabled) ota.stage = s.stage;
  if (s.autoUpdateEnabled) ota.auto_update = s.autoUpdate;
  return Object.keys(ota).length > 0 ? ota : null;
}

export function hydrateOta(record: Record<string, unknown>): HydrateResult<OtaState> {
  if (!hasOnlyKeys(record, ['stage', 'auto_update'])) {
    return { ok: false, reason: 'Template ota section contains unsupported fields.' };
  }
  const stageValue = stringField(record, 'stage');
  if (stageValue !== undefined && stageValue !== 'stable' && stageValue !== 'beta') {
    return { ok: false, reason: 'Template ota stage is not representable in the form.' };
  }
  const autoUpdateValue = stringField(record, 'auto_update');
  if (
    autoUpdateValue !== undefined &&
    autoUpdateValue !== 'off' &&
    autoUpdateValue !== 'stable' &&
    autoUpdateValue !== 'beta'
  ) {
    return { ok: false, reason: 'Template ota auto_update is not representable in the form.' };
  }
  const state = createOtaState();
  state.enabled = true;
  if (stageValue !== undefined) {
    state.stageEnabled = true;
    state.stage = stageValue;
  }
  if (autoUpdateValue !== undefined) {
    state.autoUpdateEnabled = true;
    state.autoUpdate = autoUpdateValue;
  }
  return { ok: true, state };
}

// --- auth ---

export function createAuthState(): AuthState {
  return { enabled: false, passEnabled: false, pass: '', open: false };
}

export function buildAuth(s: AuthState): Record<string, unknown> | null {
  if (!s.enabled) return null;
  const auth: Record<string, unknown> = {};
  if (s.passEnabled) auth.pass = s.pass;
  return Object.keys(auth).length > 0 ? auth : null;
}

export function hydrateAuth(record: Record<string, unknown>): HydrateResult<AuthState> {
  if (!hasOnlyKeys(record, ['pass'])) {
    return { ok: false, reason: 'Template auth section contains unsupported fields.' };
  }
  const state = createAuthState();
  state.enabled = true;
  const passValue = stringField(record, 'pass');
  if (passValue !== undefined) {
    state.passEnabled = true;
    state.pass = passValue;
  }
  return { ok: true, state };
}

// --- wifi ---

function createStaEntry(): WifiStaEntry {
  return {
    enableField: false,
    enable: true,
    ssidEnabled: false,
    ssid: '',
    passEnabled: false,
    pass: '',
    ipv4ModeEnabled: false,
    ipv4mode: 'dhcp',
    ipEnabled: false,
    ip: '',
    netmaskEnabled: false,
    netmask: '',
    gwEnabled: false,
    gw: '',
    nameserverEnabled: false,
    nameserver: '',
  };
}

function createRoamState(): WifiRoamState {
  return { rssiThrEnabled: false, rssiThr: -80, intervalEnabled: false, interval: 60 };
}

export function createWifiState(): WifiState {
  return {
    enabled: false,
    staEnabled: false,
    sta: createStaEntry(),
    sta1Enabled: false,
    sta1: createStaEntry(),
    roamEnabled: false,
    roam: createRoamState(),
    open: false,
  };
}

function buildStaEntry(s: WifiStaEntry): Record<string, unknown> {
  const sta: Record<string, unknown> = {};
  if (s.enableField) sta.enable = s.enable;
  if (s.ssidEnabled) sta.ssid = s.ssid;
  if (s.passEnabled) sta.pass = s.pass;
  if (s.ipv4ModeEnabled) sta.ipv4mode = s.ipv4mode;
  if (s.ipv4mode === 'static') {
    if (s.ipEnabled && s.ip.trim()) sta.ip = s.ip.trim();
    if (s.netmaskEnabled && s.netmask.trim()) sta.netmask = s.netmask.trim();
    if (s.gwEnabled && s.gw.trim()) sta.gw = s.gw.trim();
    if (s.nameserverEnabled && s.nameserver.trim()) sta.nameserver = s.nameserver.trim();
  }
  return sta;
}

function hydrateStaEntry(record: Record<string, unknown>): WifiStaEntry {
  const entry = createStaEntry();
  const enableValue = boolField(record, 'enable');
  if (enableValue !== undefined) {
    entry.enableField = true;
    entry.enable = enableValue;
  }
  const ssid = stringField(record, 'ssid');
  if (ssid !== undefined) {
    entry.ssidEnabled = true;
    entry.ssid = ssid;
  }
  const pass = stringField(record, 'pass');
  if (pass !== undefined) {
    entry.passEnabled = true;
    entry.pass = pass;
  }
  const ipv4mode = stringField(record, 'ipv4mode');
  if (ipv4mode !== undefined) {
    entry.ipv4ModeEnabled = true;
    entry.ipv4mode = ipv4mode === 'static' ? 'static' : 'dhcp';
  }
  const ip = stringField(record, 'ip');
  if (ip !== undefined) {
    entry.ipEnabled = true;
    entry.ip = ip;
  }
  const netmask = stringField(record, 'netmask');
  if (netmask !== undefined) {
    entry.netmaskEnabled = true;
    entry.netmask = netmask;
  }
  const gw = stringField(record, 'gw');
  if (gw !== undefined) {
    entry.gwEnabled = true;
    entry.gw = gw;
  }
  const nameserver = stringField(record, 'nameserver');
  if (nameserver !== undefined) {
    entry.nameserverEnabled = true;
    entry.nameserver = nameserver;
  }
  return entry;
}

export function buildWifi(s: WifiState): Record<string, unknown> | null {
  if (!s.enabled) return null;
  const wifi: Record<string, unknown> = {};
  if (s.staEnabled) {
    const sta = buildStaEntry(s.sta);
    if (Object.keys(sta).length > 0) wifi.sta = sta;
  }
  if (s.sta1Enabled) {
    const sta1 = buildStaEntry(s.sta1);
    if (Object.keys(sta1).length > 0) wifi.sta1 = sta1;
  }
  if (s.roamEnabled) {
    const roam: Record<string, unknown> = {};
    if (s.roam.rssiThrEnabled) roam.rssi_thr = s.roam.rssiThr;
    if (s.roam.intervalEnabled) roam.interval = s.roam.interval;
    if (Object.keys(roam).length > 0) wifi.roam = roam;
  }
  return Object.keys(wifi).length > 0 ? wifi : null;
}

export function hydrateWifi(record: Record<string, unknown>): HydrateResult<WifiState> {
  if (!hasOnlyKeys(record, ['sta', 'sta1', 'ap', 'roam'])) {
    return { ok: false, reason: 'Template wifi section contains unsupported fields.' };
  }
  const sta = record.sta ? asRecord(record.sta) : null;
  const sta1 = record.sta1 ? asRecord(record.sta1) : null;
  const roam = record.roam ? asRecord(record.roam) : null;
  if (record.sta && !sta)
    return { ok: false, reason: 'Template wifi.sta is not representable in the form.' };
  if (record.sta1 && !sta1)
    return { ok: false, reason: 'Template wifi.sta1 is not representable in the form.' };
  if (record.roam && !roam)
    return { ok: false, reason: 'Template wifi.roam is not representable in the form.' };
  const staFields = ['enable', 'ssid', 'pass', 'ipv4mode', 'ip', 'netmask', 'gw', 'nameserver'];
  if (sta && !hasOnlyKeys(sta, staFields)) {
    return { ok: false, reason: 'Template wifi.sta section contains unsupported fields.' };
  }
  if (sta1 && !hasOnlyKeys(sta1, staFields)) {
    return { ok: false, reason: 'Template wifi.sta1 section contains unsupported fields.' };
  }
  if (roam && !hasOnlyKeys(roam, ['rssi_thr', 'interval'])) {
    return { ok: false, reason: 'Template wifi.roam section contains unsupported fields.' };
  }
  const state = createWifiState();
  state.enabled = true;
  if (sta) {
    state.staEnabled = true;
    state.sta = hydrateStaEntry(sta);
  }
  if (sta1) {
    state.sta1Enabled = true;
    state.sta1 = hydrateStaEntry(sta1);
  }
  if (roam) {
    state.roamEnabled = true;
    const rssiThr = numberField(roam, 'rssi_thr');
    if (rssiThr !== undefined) {
      state.roam.rssiThrEnabled = true;
      state.roam.rssiThr = rssiThr;
    }
    const interval = numberField(roam, 'interval');
    if (interval !== undefined) {
      state.roam.intervalEnabled = true;
      state.roam.interval = interval;
    }
  }
  return { ok: true, state };
}

// --- wifi AP ---

export function createWifiAPState(): WifiAPState {
  return {
    enabled: false,
    enableField: false,
    enable: false,
    ssidEnabled: false,
    ssid: '',
    passEnabled: false,
    pass: '',
    isOpenField: false,
    isOpen: false,
    open: false,
  };
}

export function buildWifiAP(s: WifiAPState): Record<string, unknown> | null {
  if (!s.enabled) return null;
  const ap: Record<string, unknown> = {};
  if (s.enableField) ap.enable = s.enable;
  if (s.ssidEnabled) ap.ssid = s.ssid;
  if (s.passEnabled) ap.pass = s.pass;
  if (s.isOpenField) ap.is_open = s.isOpen;
  return Object.keys(ap).length > 0 ? ap : null;
}

export function hydrateWifiAP(record: Record<string, unknown>): HydrateResult<WifiAPState> {
  const ap = record.ap ? asRecord(record.ap) : null;
  if (record.ap && !ap) {
    return { ok: false, reason: 'Template wifi.ap section is not representable in the form.' };
  }
  if (ap && !hasOnlyKeys(ap, ['enable', 'ssid', 'pass', 'is_open'])) {
    return { ok: false, reason: 'Template wifi.ap section contains unsupported fields.' };
  }
  const state = createWifiAPState();
  if (!ap) return { ok: true, state };
  state.enabled = true;
  const enableValue = boolField(ap, 'enable');
  if (enableValue !== undefined) {
    state.enableField = true;
    state.enable = enableValue;
  }
  const ssidValue = stringField(ap, 'ssid');
  if (ssidValue !== undefined) {
    state.ssidEnabled = true;
    state.ssid = ssidValue;
  }
  const passValue = stringField(ap, 'pass');
  if (passValue !== undefined) {
    state.passEnabled = true;
    state.pass = passValue;
  }
  const isOpenValue = boolField(ap, 'is_open');
  if (isOpenValue !== undefined) {
    state.isOpenField = true;
    state.isOpen = isOpenValue;
  }
  return { ok: true, state };
}

// --- eth ---

export function createEthState(): EthState {
  return {
    enabled: false,
    enableField: false,
    enable: true,
    ipv4ModeEnabled: false,
    ipv4Mode: 'dhcp',
    ipEnabled: false,
    ip: '',
    netmaskEnabled: false,
    netmask: '',
    gwEnabled: false,
    gw: '',
    nameserverEnabled: false,
    nameserver: '',
    ipv6Enabled: false,
    ipv6Mode: 'disabled',
    ipv6IpEnabled: false,
    ipv6Ip: '',
    ipv6NetmaskEnabled: false,
    ipv6Netmask: '',
    ipv6GwEnabled: false,
    ipv6Gw: '',
    ipv6NameserverEnabled: false,
    ipv6Nameserver: '',
    open: false,
  };
}

export function buildEth(s: EthState): Record<string, unknown> | null {
  if (!s.enabled) return null;
  const eth: Record<string, unknown> = {};
  if (s.enableField) eth.enable = s.enable;
  if (s.ipv4ModeEnabled) eth.ipv4mode = s.ipv4Mode;
  if (s.ipEnabled && s.ip.trim() !== '') eth.ip = s.ip.trim();
  if (s.netmaskEnabled && s.netmask.trim() !== '') eth.netmask = s.netmask.trim();
  if (s.gwEnabled && s.gw.trim() !== '') eth.gw = s.gw.trim();
  if (s.nameserverEnabled && s.nameserver.trim() !== '') eth.nameserver = s.nameserver.trim();
  if (s.ipv6Enabled) {
    eth.ipv6mode = s.ipv6Mode;
    if (s.ipv6IpEnabled && s.ipv6Ip.trim() !== '') eth.ipv6_addr = s.ipv6Ip.trim();
    if (s.ipv6NetmaskEnabled && s.ipv6Netmask.trim() !== '')
      eth.ipv6_netmask = s.ipv6Netmask.trim();
    if (s.ipv6GwEnabled && s.ipv6Gw.trim() !== '') eth.ipv6_gw = s.ipv6Gw.trim();
    if (s.ipv6NameserverEnabled && s.ipv6Nameserver.trim() !== '')
      eth.ipv6_nameserver = s.ipv6Nameserver.trim();
  }
  return Object.keys(eth).length > 0 ? eth : null;
}

export function hydrateEth(record: Record<string, unknown>): HydrateResult<EthState> {
  const knownKeys = [
    'enable',
    'ipv4mode',
    'ip',
    'netmask',
    'gw',
    'nameserver',
    'ipv6mode',
    'ipv6_addr',
    'ipv6_netmask',
    'ipv6_gw',
    'ipv6_nameserver',
  ];
  if (!hasOnlyKeys(record, knownKeys)) {
    return { ok: false, reason: 'Template eth section contains unsupported fields.' };
  }
  const ipv4Mode = stringField(record, 'ipv4mode');
  if (ipv4Mode !== undefined && ipv4Mode !== 'dhcp' && ipv4Mode !== 'static') {
    return { ok: false, reason: 'Template eth ipv4mode is not representable in the form.' };
  }
  const ipv6Mode = stringField(record, 'ipv6mode');
  if (ipv6Mode !== undefined && ipv6Mode !== 'disabled' && ipv6Mode !== 'slaac') {
    return { ok: false, reason: 'Template eth ipv6mode is not representable in the form.' };
  }
  const state = createEthState();
  state.enabled = true;
  const enableValue = boolField(record, 'enable');
  if (enableValue !== undefined) {
    state.enableField = true;
    state.enable = enableValue;
  }
  if (ipv4Mode !== undefined) {
    state.ipv4ModeEnabled = true;
    state.ipv4Mode = ipv4Mode;
  }
  const ipValue = stringField(record, 'ip');
  if (ipValue !== undefined) {
    state.ipEnabled = true;
    state.ip = ipValue;
  }
  const netmaskValue = stringField(record, 'netmask');
  if (netmaskValue !== undefined) {
    state.netmaskEnabled = true;
    state.netmask = netmaskValue;
  }
  const gwValue = stringField(record, 'gw');
  if (gwValue !== undefined) {
    state.gwEnabled = true;
    state.gw = gwValue;
  }
  const nameserverValue = stringField(record, 'nameserver');
  if (nameserverValue !== undefined) {
    state.nameserverEnabled = true;
    state.nameserver = nameserverValue;
  }
  if (ipv6Mode !== undefined) {
    state.ipv6Enabled = true;
    state.ipv6Mode = ipv6Mode;
  }
  const ipv6Ip = stringField(record, 'ipv6_addr');
  if (ipv6Ip !== undefined) {
    state.ipv6Enabled = true;
    state.ipv6IpEnabled = true;
    state.ipv6Ip = ipv6Ip;
  }
  const ipv6Netmask = stringField(record, 'ipv6_netmask');
  if (ipv6Netmask !== undefined) {
    state.ipv6Enabled = true;
    state.ipv6NetmaskEnabled = true;
    state.ipv6Netmask = ipv6Netmask;
  }
  const ipv6Gw = stringField(record, 'ipv6_gw');
  if (ipv6Gw !== undefined) {
    state.ipv6Enabled = true;
    state.ipv6GwEnabled = true;
    state.ipv6Gw = ipv6Gw;
  }
  const ipv6Nameserver = stringField(record, 'ipv6_nameserver');
  if (ipv6Nameserver !== undefined) {
    state.ipv6Enabled = true;
    state.ipv6NameserverEnabled = true;
    state.ipv6Nameserver = ipv6Nameserver;
  }
  return { ok: true, state };
}

// --- modbus ---

export function createModbusState(): ModbusState {
  return { enabled: false, enableField: false, enable: false, open: false };
}

export function buildModbus(s: ModbusState): Record<string, unknown> | null {
  if (!s.enabled) return null;
  const modbus: Record<string, unknown> = {};
  if (s.enableField) modbus.enable = s.enable;
  return Object.keys(modbus).length > 0 ? modbus : null;
}

export function hydrateModbus(record: Record<string, unknown>): HydrateResult<ModbusState> {
  if (!hasOnlyKeys(record, ['enable'])) {
    return { ok: false, reason: 'Template modbus section contains unsupported fields.' };
  }
  const state = createModbusState();
  state.enabled = true;
  const enableValue = boolField(record, 'enable');
  if (enableValue !== undefined) {
    state.enableField = true;
    state.enable = enableValue;
  }
  return { ok: true, state };
}

// --- zigbee ---

export function createZigbeeState(): ZigbeeState {
  return { enabled: false, enableField: false, enable: false, open: false };
}

export function buildZigbee(s: ZigbeeState): Record<string, unknown> | null {
  if (!s.enabled) return null;
  const zigbee: Record<string, unknown> = {};
  if (s.enableField) zigbee.enable = s.enable;
  return Object.keys(zigbee).length > 0 ? zigbee : null;
}

export function hydrateZigbee(record: Record<string, unknown>): HydrateResult<ZigbeeState> {
  if (!hasOnlyKeys(record, ['enable'])) {
    return { ok: false, reason: 'Template zigbee section contains unsupported fields.' };
  }
  const state = createZigbeeState();
  state.enabled = true;
  const enableValue = boolField(record, 'enable');
  if (enableValue !== undefined) {
    state.enableField = true;
    state.enable = enableValue;
  }
  return { ok: true, state };
}

// --- ui ---

export function createUIState(): UIState {
  return { enabled: false, idleBrightnessEnabled: false, idleBrightness: 30, open: false };
}

export function buildUI(s: UIState): Record<string, unknown> | null {
  if (!s.enabled) return null;
  const ui: Record<string, unknown> = {};
  if (s.idleBrightnessEnabled) ui.idle_brightness = s.idleBrightness;
  return Object.keys(ui).length > 0 ? ui : null;
}

export function hydrateUI(record: Record<string, unknown>): HydrateResult<UIState> {
  if (!hasOnlyKeys(record, ['idle_brightness'])) {
    return { ok: false, reason: 'Template ui section contains unsupported fields.' };
  }
  const state = createUIState();
  state.enabled = true;
  const brightness = numberField(record, 'idle_brightness');
  if (brightness !== undefined) {
    state.idleBrightnessEnabled = true;
    state.idleBrightness = brightness;
  }
  return { ok: true, state };
}

// --- script ---

export function createScriptsState(): ScriptsState {
  return { enabled: false, scripts: [], open: false };
}

export function buildScripts(s: ScriptsState): Record<string, unknown> | null {
  if (!s.enabled || s.scripts.length === 0) return null;
  const out: Record<string, unknown> = {};
  for (const entry of s.scripts) {
    if (entry.id.trim() === '') continue;
    out[entry.id.trim()] = { name: entry.name, enable: entry.enable };
  }
  return Object.keys(out).length > 0 ? out : null;
}

export function hydrateScripts(record: Record<string, unknown>): HydrateResult<ScriptsState> {
  const scripts: ScriptEntry[] = [];
  for (const [key, val] of Object.entries(record)) {
    if (!/^\d+$/.test(key)) {
      return { ok: false, reason: `Template script section key "${key}" is not a numeric id.` };
    }
    const cfg = asRecord(val);
    if (!cfg) {
      return { ok: false, reason: `Template script ${key} config must be an object.` };
    }
    if (!hasOnlyKeys(cfg, ['name', 'enable'])) {
      return { ok: false, reason: `Template script ${key} contains unsupported fields.` };
    }
    const name = stringField(cfg, 'name') ?? '';
    const enable = boolField(cfg, 'enable') ?? true;
    scripts.push({ id: key, name, enable });
  }
  return { ok: true, state: { enabled: true, scripts, open: false } };
}
