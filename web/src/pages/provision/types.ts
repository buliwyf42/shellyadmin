export type HydrateResult<T> = { ok: true; state: T } | { ok: false; reason: string }

export type SysState = {
  enabled: boolean
  nameEnabled: boolean
  name: string
  tzEnabled: boolean
  tz: string
  latEnabled: boolean
  lat: string
  lonEnabled: boolean
  lon: string
  sntpEnabled: boolean
  sntp: string
  debugWSEnabled: boolean
  debugWS: boolean
  debugMQTTEnabled: boolean
  debugMQTT: boolean
  debugUDPHostEnabled: boolean
  debugUDPHost: string
  rpcUDPPortEnabled: boolean
  rpcUDPPort: string
  ecoEnabled: boolean
  eco: boolean
  discoverableEnabled: boolean
  discoverable: boolean
  open: boolean
}

export type MqttState = {
  enabled: boolean
  enableField: boolean
  enable: boolean
  serverEnabled: boolean
  server: string
  clientIDEnabled: boolean
  clientID: string
  topicPrefixEnabled: boolean
  topicPrefix: string
  userEnabled: boolean
  user: string
  passEnabled: boolean
  pass: string
  sslCAEnabled: boolean
  sslCA: string
  rpcNtfEnabled: boolean
  rpcNtf: boolean
  statusNtfEnabled: boolean
  statusNtf: boolean
  enableRPCEnabled: boolean
  enableRPC: boolean
  enableControlEnabled: boolean
  enableControl: boolean
  useClientCertEnabled: boolean
  useClientCert: boolean
  open: boolean
}

export type WsState = {
  enabled: boolean
  enableField: boolean
  enable: boolean
  serverEnabled: boolean
  server: string
  tlsModeEnabled: boolean
  tlsMode: 'no_validation' | 'default' | 'user'
  sslCAEnabled: boolean
  sslCA: string
  open: boolean
}

export type BleState = {
  enabled: boolean
  enableField: boolean
  enable: boolean
  rpcEnabledField: boolean
  rpcEnabled: boolean
  observerEnabledField: boolean
  observerEnabled: boolean
  open: boolean
}

export type MatterState = {
  enabled: boolean
  enableField: boolean
  enable: boolean
  open: boolean
}

export type CloudState = {
  enabled: boolean
  enableField: boolean
  enable: boolean
  open: boolean
}

export type OtaState = {
  enabled: boolean
  stageEnabled: boolean
  stage: 'stable' | 'beta'
  autoUpdateEnabled: boolean
  autoUpdate: 'off' | 'stable' | 'beta'
  open: boolean
}

export type AuthState = {
  enabled: boolean
  passEnabled: boolean
  pass: string
  open: boolean
}

export type WifiStaEntry = {
  enableField: boolean
  enable: boolean
  ssidEnabled: boolean
  ssid: string
  passEnabled: boolean
  pass: string
  ipv4ModeEnabled: boolean
  ipv4mode: 'dhcp' | 'static'
  ipEnabled: boolean
  ip: string
  netmaskEnabled: boolean
  netmask: string
  gwEnabled: boolean
  gw: string
  nameserverEnabled: boolean
  nameserver: string
}

export type WifiRoamState = {
  rssiThrEnabled: boolean
  rssiThr: number
  intervalEnabled: boolean
  interval: number
}

export type WifiState = {
  enabled: boolean
  staEnabled: boolean
  sta: WifiStaEntry
  sta1Enabled: boolean
  sta1: WifiStaEntry
  roamEnabled: boolean
  roam: WifiRoamState
  open: boolean
}

export type WifiAPState = {
  enabled: boolean
  enableField: boolean
  enable: boolean
  ssidEnabled: boolean
  ssid: string
  passEnabled: boolean
  pass: string
  isOpenField: boolean
  isOpen: boolean
  open: boolean
}

export type ModbusState = {
  enabled: boolean
  enableField: boolean
  enable: boolean
  open: boolean
}

export type ZigbeeState = {
  enabled: boolean
  enableField: boolean
  enable: boolean
  open: boolean
}

export type EthState = {
  enabled: boolean
  enableField: boolean
  enable: boolean
  ipv4ModeEnabled: boolean
  ipv4Mode: 'dhcp' | 'static'
  ipEnabled: boolean
  ip: string
  netmaskEnabled: boolean
  netmask: string
  gwEnabled: boolean
  gw: string
  nameserverEnabled: boolean
  nameserver: string
  ipv6Enabled: boolean
  ipv6Mode: 'disabled' | 'slaac'
  ipv6IpEnabled: boolean
  ipv6Ip: string
  ipv6NetmaskEnabled: boolean
  ipv6Netmask: string
  ipv6GwEnabled: boolean
  ipv6Gw: string
  ipv6NameserverEnabled: boolean
  ipv6Nameserver: string
  open: boolean
}

export type UIState = {
  enabled: boolean
  idleBrightnessEnabled: boolean
  idleBrightness: number
  open: boolean
}

export type ScriptEntry = {
  id: string
  name: string
  enable: boolean
}

export type ScriptsState = {
  enabled: boolean
  scripts: ScriptEntry[]
  open: boolean
}
