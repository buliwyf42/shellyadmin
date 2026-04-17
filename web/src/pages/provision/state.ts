import type {
  AuthState,
  BleState,
  CloudState,
  HydrateResult,
  MatterState,
  MqttState,
  OtaState,
  SysState,
  WifiState,
  WsState,
} from './types'

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : null
}

function hasOnlyKeys(record: Record<string, unknown>, keys: string[]): boolean {
  return Object.keys(record).every((key) => keys.includes(key))
}

function boolField(record: Record<string, unknown>, key: string): boolean | undefined {
  const value = record[key]
  return typeof value === 'boolean' ? value : undefined
}

function stringField(record: Record<string, unknown>, key: string): string | undefined {
  const value = record[key]
  return typeof value === 'string' ? value : undefined
}

function numberField(record: Record<string, unknown>, key: string): number | undefined {
  const value = record[key]
  return typeof value === 'number' ? value : undefined
}

function maybeNum(raw: string | number): number | undefined {
  if (typeof raw === 'number') return Number.isFinite(raw) ? raw : undefined
  if (raw.trim() === '') return undefined
  const n = Number(raw)
  return Number.isFinite(n) ? n : undefined
}

export function isTLSServerURL(raw: string): boolean {
  return raw.trim().toLowerCase().startsWith('wss://')
}

function inferWSTLSMode(
  server: string | undefined,
  sslCA: string | undefined,
  explicitMode: string | undefined,
): 'no_validation' | 'default' | 'user' | undefined {
  if (explicitMode === 'no_validation' || explicitMode === 'default' || explicitMode === 'user') return explicitMode
  if (!server || !isTLSServerURL(server)) return undefined
  if (sslCA === '*') return 'no_validation'
  if (sslCA && sslCA.trim() !== '') return 'user'
  return 'default'
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
    debugUDPHostEnabled: false,
    debugUDPHost: '',
    rpcUDPPortEnabled: false,
    rpcUDPPort: '0',
    ecoEnabled: false,
    eco: false,
    discoverableEnabled: false,
    discoverable: true,
    open: false,
  }
}

export function buildSys(s: SysState): Record<string, unknown> | null {
  if (!s.enabled) return null
  const sys: Record<string, unknown> = {}
  const deviceCfg: Record<string, unknown> = {}
  const location: Record<string, unknown> = {}
  const sntp: Record<string, unknown> = {}
  const debug: Record<string, unknown> = {}
  const debugWS: Record<string, unknown> = {}
  const debugUDP: Record<string, unknown> = {}
  const rpcUDP: Record<string, unknown> = {}

  if (s.nameEnabled) deviceCfg.name = s.name
  if (s.ecoEnabled) deviceCfg.eco_mode = s.eco
  if (s.discoverableEnabled) deviceCfg.discoverable = s.discoverable
  if (s.tzEnabled) location.tz = s.tz
  if (s.sntpEnabled) sntp.server = s.sntp
  if (s.debugWSEnabled) debugWS.enable = s.debugWS
  if (s.debugUDPHostEnabled && s.debugUDPHost.trim()) debugUDP.addr = s.debugUDPHost.trim()
  if (s.rpcUDPPortEnabled) {
    const port = maybeNum(s.rpcUDPPort)
    rpcUDP.listen_port = port === undefined ? 0 : port
  }
  if (s.latEnabled) {
    const lat = maybeNum(s.lat)
    if (lat !== undefined) location.lat = lat
  }
  if (s.lonEnabled) {
    const lon = maybeNum(s.lon)
    if (lon !== undefined) location.lon = lon
  }
  if (Object.keys(deviceCfg).length > 0) sys.device = deviceCfg
  if (Object.keys(location).length > 0) sys.location = location
  if (Object.keys(sntp).length > 0) sys.sntp = sntp
  if (Object.keys(debugWS).length > 0) debug.websocket = debugWS
  if (Object.keys(debugUDP).length > 0) debug.udp = debugUDP
  if (Object.keys(debug).length > 0) sys.debug = debug
  if (Object.keys(rpcUDP).length > 0) sys.rpc_udp = rpcUDP
  return Object.keys(sys).length > 0 ? sys : null
}

export function hydrateSys(record: Record<string, unknown>): HydrateResult<SysState> {
  if (!hasOnlyKeys(record, ['name', 'device', 'tz', 'location', 'sntp', 'dbg', 'debug', 'rpc_udp', 'lat', 'lng', 'lon', 'profile', 'addon_type'])) {
    return { ok: false, reason: 'Template sys section contains fields the form cannot represent.' }
  }
  const device = record.device ? asRecord(record.device) : null
  const location = record.location ? asRecord(record.location) : null
  const sntp = record.sntp ? asRecord(record.sntp) : null
  const dbg = record.dbg ? asRecord(record.dbg) : null
  const debug = record.debug ? asRecord(record.debug) : null
  const rpcUDP = record.rpc_udp ? asRecord(record.rpc_udp) : null
  if ((record.device && !device) || (record.location && !location) || (record.sntp && !sntp) || (record.dbg && !dbg) || (record.debug && !debug) || (record.rpc_udp && !rpcUDP)) {
    return { ok: false, reason: 'Template sys section contains nested values the form cannot represent.' }
  }
  if (device && !hasOnlyKeys(device, ['name', 'eco_mode', 'discoverable'])) {
    return { ok: false, reason: 'Template sys.device section contains unsupported fields.' }
  }
  if (location && !hasOnlyKeys(location, ['tz', 'lat', 'lon'])) {
    return { ok: false, reason: 'Template sys.location section contains unsupported fields.' }
  }
  if (sntp && !hasOnlyKeys(sntp, ['server'])) {
    return { ok: false, reason: 'Template sys.sntp section contains unsupported fields.' }
  }
  if (dbg && !hasOnlyKeys(dbg, ['websocket_enable', 'udp_addr'])) {
    return { ok: false, reason: 'Template sys.dbg section contains unsupported fields.' }
  }
  const debugWS = debug && debug.websocket ? asRecord(debug.websocket) : null
  const debugUDP = debug && debug.udp ? asRecord(debug.udp) : null
  if ((debug && debug.websocket && !debugWS) || (debug && debug.udp && !debugUDP)) {
    return { ok: false, reason: 'Template sys.debug section contains unsupported nested values.' }
  }
  if (debugWS && !hasOnlyKeys(debugWS, ['enable'])) {
    return { ok: false, reason: 'Template sys.debug.websocket section contains unsupported fields.' }
  }
  if (debugUDP && !hasOnlyKeys(debugUDP, ['addr'])) {
    return { ok: false, reason: 'Template sys.debug.udp section contains unsupported fields.' }
  }
  if (rpcUDP && !hasOnlyKeys(rpcUDP, ['port', 'listen_port'])) {
    return { ok: false, reason: 'Template sys.rpc_udp section contains unsupported fields.' }
  }

  const state = createSysState()
  state.enabled = true
  const topName = stringField(record, 'name')
  const nestedName = device ? stringField(device, 'name') : undefined
  if (topName !== undefined || nestedName !== undefined) {
    if (topName !== undefined && nestedName !== undefined && topName !== nestedName) {
      return { ok: false, reason: 'Template sys name fields disagree and cannot be represented safely in the form.' }
    }
    state.nameEnabled = true
    state.name = topName ?? nestedName ?? state.name
  }
  const topTZ = stringField(record, 'tz')
  const nestedTZ = location ? stringField(location, 'tz') : undefined
  if (topTZ !== undefined || nestedTZ !== undefined) {
    if (topTZ !== undefined && nestedTZ !== undefined && topTZ !== nestedTZ) {
      return { ok: false, reason: 'Template sys timezone fields disagree and cannot be represented safely in the form.' }
    }
    state.tzEnabled = true
    state.tz = topTZ ?? nestedTZ ?? state.tz
  }
  const topLat = numberField(record, 'lat')
  const nestedLat = location ? numberField(location, 'lat') : undefined
  if (topLat !== undefined || nestedLat !== undefined) {
    if (topLat !== undefined && nestedLat !== undefined && topLat !== nestedLat) {
      return { ok: false, reason: 'Template sys latitude fields disagree and cannot be represented safely in the form.' }
    }
    state.latEnabled = true
    state.lat = String(topLat ?? nestedLat ?? '')
  }
  const topLon = numberField(record, 'lng') ?? numberField(record, 'lon')
  const nestedLon = location ? numberField(location, 'lon') : undefined
  if (topLon !== undefined || nestedLon !== undefined) {
    if (topLon !== undefined && nestedLon !== undefined && topLon !== nestedLon) {
      return { ok: false, reason: 'Template sys longitude fields disagree and cannot be represented safely in the form.' }
    }
    state.lonEnabled = true
    state.lon = String(topLon ?? nestedLon ?? '')
  }
  const sntpServer = sntp ? stringField(sntp, 'server') : undefined
  if (sntpServer !== undefined) {
    state.sntpEnabled = true
    state.sntp = sntpServer
  }
  const legacyDebugWS = dbg ? boolField(dbg, 'websocket_enable') : undefined
  const nestedDebugWebsocket = debugWS ? boolField(debugWS, 'enable') : undefined
  const finalDebugWS = legacyDebugWS !== undefined ? legacyDebugWS : nestedDebugWebsocket
  if (finalDebugWS !== undefined) {
    state.debugWSEnabled = true
    state.debugWS = finalDebugWS
  }
  const legacyDebugUDPHost = dbg ? stringField(dbg, 'udp_addr') : undefined
  const nestedDebugUDPHost = debugUDP ? stringField(debugUDP, 'addr') : undefined
  const debugUDPHost = legacyDebugUDPHost ?? nestedDebugUDPHost
  if (debugUDPHost !== undefined) {
    state.debugUDPHostEnabled = true
    state.debugUDPHost = debugUDPHost
  }
  const rpcUDPPort = rpcUDP ? (numberField(rpcUDP, 'listen_port') ?? numberField(rpcUDP, 'port')) : undefined
  if (rpcUDPPort !== undefined) {
    state.rpcUDPPortEnabled = true
    state.rpcUDPPort = String(rpcUDPPort)
  }
  const ecoMode = device ? boolField(device, 'eco_mode') : undefined
  if (ecoMode !== undefined) {
    state.ecoEnabled = true
    state.eco = ecoMode
  }
  const discoverable = device ? boolField(device, 'discoverable') : undefined
  if (discoverable !== undefined) {
    state.discoverableEnabled = true
    state.discoverable = discoverable
  }
  return { ok: true, state }
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
  }
}

export function buildMqtt(s: MqttState): Record<string, unknown> | null {
  if (!s.enabled) return null
  const mqtt: Record<string, unknown> = {}
  if (s.enableField) mqtt.enable = s.enable
  if (s.serverEnabled) mqtt.server = s.server
  if (s.clientIDEnabled) {
    mqtt.client_id = s.clientID
    mqtt.id = s.clientID
  }
  if (s.topicPrefixEnabled) mqtt.topic_prefix = s.topicPrefix
  if (s.userEnabled) mqtt.user = s.user
  if (s.passEnabled) mqtt.pass = s.pass
  if (s.sslCAEnabled && s.sslCA !== '') mqtt.ssl_ca = s.sslCA
  if (s.rpcNtfEnabled) mqtt.rpc_ntf = s.rpcNtf
  if (s.statusNtfEnabled) mqtt.status_ntf = s.statusNtf
  if (s.enableRPCEnabled) mqtt.enable_rpc = s.enableRPC
  if (s.enableControlEnabled) mqtt.enable_control = s.enableControl
  if (s.useClientCertEnabled) mqtt.use_client_cert = s.useClientCert
  return Object.keys(mqtt).length > 0 ? mqtt : null
}

export function hydrateMqtt(record: Record<string, unknown>): HydrateResult<MqttState> {
  if (!hasOnlyKeys(record, ['enable', 'server', 'client_id', 'id', 'topic_prefix', 'user', 'pass', 'ssl_ca', 'rpc_ntf', 'status_ntf', 'enable_rpc', 'enable_control', 'use_client_cert'])) {
    return { ok: false, reason: 'Template mqtt section contains unsupported fields.' }
  }
  const clientID = stringField(record, 'client_id')
  const aliasID = stringField(record, 'id')
  if (clientID !== undefined && aliasID !== undefined && clientID !== aliasID) {
    return { ok: false, reason: 'Template mqtt client identifiers disagree and cannot be represented safely in the form.' }
  }
  const state = createMqttState()
  state.enabled = true
  const enableValue = boolField(record, 'enable')
  if (enableValue !== undefined) {
    state.enableField = true
    state.enable = enableValue
  }
  const serverValue = stringField(record, 'server')
  if (serverValue !== undefined) {
    state.serverEnabled = true
    state.server = serverValue
  }
  const clientValue = clientID ?? aliasID
  if (clientValue !== undefined) {
    state.clientIDEnabled = true
    state.clientID = clientValue
  }
  const topicValue = stringField(record, 'topic_prefix')
  if (topicValue !== undefined) {
    state.topicPrefixEnabled = true
    state.topicPrefix = topicValue
  }
  const userValue = stringField(record, 'user')
  if (userValue !== undefined) {
    state.userEnabled = true
    state.user = userValue
  }
  const passValue = stringField(record, 'pass')
  if (passValue !== undefined) {
    state.passEnabled = true
    state.pass = passValue
  }
  const sslCAValue = stringField(record, 'ssl_ca')
  if (sslCAValue !== undefined) {
    state.sslCAEnabled = true
    state.sslCA = sslCAValue
  }
  const rpcValue = boolField(record, 'rpc_ntf')
  if (rpcValue !== undefined) {
    state.rpcNtfEnabled = true
    state.rpcNtf = rpcValue
  }
  const statusValue = boolField(record, 'status_ntf')
  if (statusValue !== undefined) {
    state.statusNtfEnabled = true
    state.statusNtf = statusValue
  }
  const enableRPCValue = boolField(record, 'enable_rpc')
  if (enableRPCValue !== undefined) {
    state.enableRPCEnabled = true
    state.enableRPC = enableRPCValue
  }
  const enableControlValue = boolField(record, 'enable_control')
  if (enableControlValue !== undefined) {
    state.enableControlEnabled = true
    state.enableControl = enableControlValue
  }
  const useClientCertValue = boolField(record, 'use_client_cert')
  if (useClientCertValue !== undefined) {
    state.useClientCertEnabled = true
    state.useClientCert = useClientCertValue
  }
  return { ok: true, state }
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
  }
}

export function buildWs(s: WsState): Record<string, unknown> | null {
  if (!s.enabled) return null
  const ws: Record<string, unknown> = {}
  if (s.enableField) ws.enable = s.enable
  if (s.serverEnabled) ws.server = s.server
  if (isTLSServerURL(s.server)) {
    if (s.tlsModeEnabled) ws.tls_mode = s.tlsMode
    if (s.sslCAEnabled && s.tlsMode === 'user') ws.ssl_ca = s.sslCA
  }
  return Object.keys(ws).length > 0 ? ws : null
}

export function hydrateWs(record: Record<string, unknown>): HydrateResult<WsState> {
  if (!hasOnlyKeys(record, ['enable', 'server', 'tls_mode', 'ssl_ca'])) {
    return { ok: false, reason: 'Template ws section contains unsupported fields.' }
  }
  const tlsMode = stringField(record, 'tls_mode')
  if (tlsMode !== undefined && tlsMode !== 'no_validation' && tlsMode !== 'default' && tlsMode !== 'user') {
    return { ok: false, reason: 'Template ws tls_mode is not representable in the form.' }
  }
  const state = createWsState()
  state.enabled = true
  const enableValue = boolField(record, 'enable')
  if (enableValue !== undefined) {
    state.enableField = true
    state.enable = enableValue
  }
  const serverValue = stringField(record, 'server')
  if (serverValue !== undefined) {
    state.serverEnabled = true
    state.server = serverValue
  }
  const sslCAValue = stringField(record, 'ssl_ca')
  const inferredTLSMode = inferWSTLSMode(serverValue, sslCAValue, tlsMode)
  if (inferredTLSMode !== undefined) {
    state.tlsModeEnabled = true
    state.tlsMode = inferredTLSMode
  }
  if (sslCAValue !== undefined && sslCAValue !== '*') {
    state.sslCAEnabled = true
    state.sslCA = sslCAValue
  }
  return { ok: true, state }
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
  }
}

export function buildBle(s: BleState): Record<string, unknown> | null {
  if (!s.enabled) return null
  const ble: Record<string, unknown> = {}
  if (s.enableField) ble.enable = s.enable
  if (s.rpcEnabledField) ble.rpc = { enable: s.rpcEnabled }
  if (s.observerEnabledField) ble.observer = { enable: s.observerEnabled }
  return Object.keys(ble).length > 0 ? ble : null
}

export function hydrateBle(record: Record<string, unknown>): HydrateResult<BleState> {
  if (!hasOnlyKeys(record, ['enable', 'rpc', 'observer'])) {
    return { ok: false, reason: 'Template ble section contains unsupported fields.' }
  }
  const rpc = record.rpc ? asRecord(record.rpc) : null
  const observer = record.observer ? asRecord(record.observer) : null
  if ((record.rpc && !rpc) || (record.observer && !observer)) {
    return { ok: false, reason: 'Template ble section contains nested values the form cannot represent.' }
  }
  if (rpc && !hasOnlyKeys(rpc, ['enable'])) {
    return { ok: false, reason: 'Template ble.rpc section contains unsupported fields.' }
  }
  if (observer && !hasOnlyKeys(observer, ['enable'])) {
    return { ok: false, reason: 'Template ble.observer section contains unsupported fields.' }
  }
  const state = createBleState()
  state.enabled = true
  const enableValue = boolField(record, 'enable')
  if (enableValue !== undefined) {
    state.enableField = true
    state.enable = enableValue
  }
  const rpcValue = rpc ? boolField(rpc, 'enable') : undefined
  if (rpcValue !== undefined) {
    state.rpcEnabledField = true
    state.rpcEnabled = rpcValue
  }
  const observerValue = observer ? boolField(observer, 'enable') : undefined
  if (observerValue !== undefined) {
    state.observerEnabledField = true
    state.observerEnabled = observerValue
  }
  return { ok: true, state }
}

// --- matter ---

export function createMatterState(): MatterState {
  return { enabled: false, enableField: false, enable: true, open: false }
}

export function buildMatter(s: MatterState): Record<string, unknown> | null {
  if (!s.enabled) return null
  const matter: Record<string, unknown> = {}
  if (s.enableField) matter.enable = s.enable
  return Object.keys(matter).length > 0 ? matter : null
}

export function hydrateMatter(record: Record<string, unknown>): HydrateResult<MatterState> {
  if (!hasOnlyKeys(record, ['enable'])) {
    return { ok: false, reason: 'Template matter section contains unsupported fields.' }
  }
  const state = createMatterState()
  state.enabled = true
  const enableValue = boolField(record, 'enable')
  if (enableValue !== undefined) {
    state.enableField = true
    state.enable = enableValue
  }
  return { ok: true, state }
}

// --- cloud ---

export function createCloudState(): CloudState {
  return { enabled: false, enableField: false, enable: true, open: false }
}

export function buildCloud(s: CloudState): Record<string, unknown> | null {
  if (!s.enabled) return null
  const cloud: Record<string, unknown> = {}
  if (s.enableField) cloud.enable = s.enable
  return Object.keys(cloud).length > 0 ? cloud : null
}

export function hydrateCloud(record: Record<string, unknown>): HydrateResult<CloudState> {
  if (!hasOnlyKeys(record, ['enable'])) {
    return { ok: false, reason: 'Template cloud section contains unsupported fields.' }
  }
  const state = createCloudState()
  state.enabled = true
  const enableValue = boolField(record, 'enable')
  if (enableValue !== undefined) {
    state.enableField = true
    state.enable = enableValue
  }
  return { ok: true, state }
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
  }
}

export function buildOta(s: OtaState): Record<string, unknown> | null {
  if (!s.enabled) return null
  const ota: Record<string, unknown> = {}
  if (s.stageEnabled) ota.stage = s.stage
  if (s.autoUpdateEnabled) ota.auto_update = s.autoUpdate
  return Object.keys(ota).length > 0 ? ota : null
}

export function hydrateOta(record: Record<string, unknown>): HydrateResult<OtaState> {
  if (!hasOnlyKeys(record, ['stage', 'auto_update'])) {
    return { ok: false, reason: 'Template ota section contains unsupported fields.' }
  }
  const stageValue = stringField(record, 'stage')
  if (stageValue !== undefined && stageValue !== 'stable' && stageValue !== 'beta') {
    return { ok: false, reason: 'Template ota stage is not representable in the form.' }
  }
  const autoUpdateValue = stringField(record, 'auto_update')
  if (autoUpdateValue !== undefined && autoUpdateValue !== 'off' && autoUpdateValue !== 'stable' && autoUpdateValue !== 'beta') {
    return { ok: false, reason: 'Template ota auto_update is not representable in the form.' }
  }
  const state = createOtaState()
  state.enabled = true
  if (stageValue !== undefined) {
    state.stageEnabled = true
    state.stage = stageValue
  }
  if (autoUpdateValue !== undefined) {
    state.autoUpdateEnabled = true
    state.autoUpdate = autoUpdateValue
  }
  return { ok: true, state }
}

// --- auth ---

export function createAuthState(): AuthState {
  return { enabled: false, passEnabled: false, pass: '', open: false }
}

export function buildAuth(s: AuthState): Record<string, unknown> | null {
  if (!s.enabled) return null
  const auth: Record<string, unknown> = {}
  if (s.passEnabled) auth.pass = s.pass
  return Object.keys(auth).length > 0 ? auth : null
}

export function hydrateAuth(record: Record<string, unknown>): HydrateResult<AuthState> {
  if (!hasOnlyKeys(record, ['pass'])) {
    return { ok: false, reason: 'Template auth section contains unsupported fields.' }
  }
  const state = createAuthState()
  state.enabled = true
  const passValue = stringField(record, 'pass')
  if (passValue !== undefined) {
    state.passEnabled = true
    state.pass = passValue
  }
  return { ok: true, state }
}

// --- wifi ---

export function createWifiState(): WifiState {
  return {
    enabled: false,
    staEnabled: false,
    ssidEnabled: false,
    ssid: '',
    passEnabled: false,
    pass: '',
    open: false,
  }
}

export function buildWifi(s: WifiState): Record<string, unknown> | null {
  if (!s.enabled) return null
  const wifi: Record<string, unknown> = {}
  const sta: Record<string, unknown> = {}
  if (s.staEnabled) sta.enable = true
  if (s.ssidEnabled) sta.ssid = s.ssid
  if (s.passEnabled) sta.pass = s.pass
  if (Object.keys(sta).length > 0) wifi.sta = sta
  return Object.keys(wifi).length > 0 ? wifi : null
}

export function hydrateWifi(record: Record<string, unknown>): HydrateResult<WifiState> {
  if (!hasOnlyKeys(record, ['sta'])) {
    return { ok: false, reason: 'Template wifi section contains unsupported fields.' }
  }
  const sta = record.sta ? asRecord(record.sta) : null
  if (record.sta && !sta) {
    return { ok: false, reason: 'Template wifi.sta section is not representable in the form.' }
  }
  if (sta && !hasOnlyKeys(sta, ['enable', 'ssid', 'pass'])) {
    return { ok: false, reason: 'Template wifi.sta section contains unsupported fields.' }
  }
  const state = createWifiState()
  state.enabled = true
  const staEnabled = sta ? boolField(sta, 'enable') : undefined
  if (staEnabled !== undefined) state.staEnabled = staEnabled
  const ssidValue = sta ? stringField(sta, 'ssid') : undefined
  if (ssidValue !== undefined) {
    state.ssidEnabled = true
    state.ssid = ssidValue
  }
  const passValue = sta ? stringField(sta, 'pass') : undefined
  if (passValue !== undefined) {
    state.passEnabled = true
    state.pass = passValue
  }
  return { ok: true, state }
}
