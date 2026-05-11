export type HydrateResult<T> = { ok: true; state: T } | { ok: false; reason: string };

export type SysState = {
  nameEnabled: boolean;
  name: string;
  tzEnabled: boolean;
  tz: string;
  latEnabled: boolean;
  lat: string;
  lonEnabled: boolean;
  lon: string;
  sntpEnabled: boolean;
  sntp: string;
  debugWSEnabled: boolean;
  debugWS: boolean;
  debugMQTTEnabled: boolean;
  debugMQTT: boolean;
  debugUDPHostEnabled: boolean;
  debugUDPHost: string;
  rpcUDPPortEnabled: boolean;
  rpcUDPPort: string;
  ecoEnabled: boolean;
  eco: boolean;
  discoverableEnabled: boolean;
  discoverable: boolean;
  open: boolean;
};

export type MqttState = {
  enableField: boolean;
  enable: boolean;
  serverEnabled: boolean;
  server: string;
  clientIDEnabled: boolean;
  clientID: string;
  topicPrefixEnabled: boolean;
  topicPrefix: string;
  userEnabled: boolean;
  user: string;
  passEnabled: boolean;
  pass: string;
  sslCAEnabled: boolean;
  sslCA: string;
  rpcNtfEnabled: boolean;
  rpcNtf: boolean;
  statusNtfEnabled: boolean;
  statusNtf: boolean;
  enableRPCEnabled: boolean;
  enableRPC: boolean;
  enableControlEnabled: boolean;
  enableControl: boolean;
  useClientCertEnabled: boolean;
  useClientCert: boolean;
  open: boolean;
};

export type WsState = {
  enableField: boolean;
  enable: boolean;
  serverEnabled: boolean;
  server: string;
  tlsModeEnabled: boolean;
  tlsMode: 'no_validation' | 'default' | 'user';
  sslCAEnabled: boolean;
  sslCA: string;
  open: boolean;
};

// BleState no longer carries the global enable flag — Shelly firmware 2.0.0-beta1
// removed `ble.enable` and BLE auto-activates with scanning. We still hydrate
// older templates by silently dropping the field; see hydrateBle in state.ts.
export type BleState = {
  rpcEnabledField: boolean;
  rpcEnabled: boolean;
  observerEnabledField: boolean;
  observerEnabled: boolean;
  open: boolean;
};

export type MatterState = {
  enableField: boolean;
  enable: boolean;
  open: boolean;
};

export type AutoUpdateState = {
  enabled: boolean;
  mode: 'off' | 'stable' | 'beta';
  open: boolean;
};

export type CloudState = {
  enableField: boolean;
  enable: boolean;
  open: boolean;
};

export type AuthState = {
  passEnabled: boolean;
  pass: string;
  open: boolean;
};

export type WifiStaEntry = {
  enableField: boolean;
  enable: boolean;
  ssidEnabled: boolean;
  ssid: string;
  passEnabled: boolean;
  pass: string;
  ipv4ModeEnabled: boolean;
  ipv4mode: 'dhcp' | 'static';
  ipEnabled: boolean;
  ip: string;
  netmaskEnabled: boolean;
  netmask: string;
  gwEnabled: boolean;
  gw: string;
  nameserverEnabled: boolean;
  nameserver: string;
  // Firmware 2.0.0-beta1: per-device hostname configuration.
  hostnameEnabled: boolean;
  hostname: string;
};

export type WifiRoamState = {
  rssiThrEnabled: boolean;
  rssiThr: number;
  intervalEnabled: boolean;
  interval: number;
};

export type WifiState = {
  staEnabled: boolean;
  sta: WifiStaEntry;
  sta1Enabled: boolean;
  sta1: WifiStaEntry;
  roamEnabled: boolean;
  roam: WifiRoamState;
  open: boolean;
};

export type WifiAPState = {
  enableField: boolean;
  enable: boolean;
  ssidEnabled: boolean;
  ssid: string;
  passEnabled: boolean;
  pass: string;
  isOpenField: boolean;
  isOpen: boolean;
  open: boolean;
};

export type ModbusState = {
  enableField: boolean;
  enable: boolean;
  open: boolean;
};

export type ZigbeeState = {
  enableField: boolean;
  enable: boolean;
  open: boolean;
};

export type EthState = {
  enableField: boolean;
  enable: boolean;
  ipv4ModeEnabled: boolean;
  ipv4Mode: 'dhcp' | 'static';
  ipEnabled: boolean;
  ip: string;
  netmaskEnabled: boolean;
  netmask: string;
  gwEnabled: boolean;
  gw: string;
  nameserverEnabled: boolean;
  nameserver: string;
  ipv6Enabled: boolean;
  ipv6Mode: 'disabled' | 'slaac';
  ipv6IpEnabled: boolean;
  ipv6Ip: string;
  ipv6NetmaskEnabled: boolean;
  ipv6Netmask: string;
  ipv6GwEnabled: boolean;
  ipv6Gw: string;
  ipv6NameserverEnabled: boolean;
  ipv6Nameserver: string;
  open: boolean;
};

export type UIState = {
  idleBrightnessEnabled: boolean;
  idleBrightness: number;
  open: boolean;
};

export type ScriptEntry = {
  id: string;
  name: string;
  enable: boolean;
};

export type ScriptsState = {
  scripts: ScriptEntry[];
  open: boolean;
};

// Webhooks form covers the common case (wipe + create) for the
// `webhooks` provisioner section. Updates of existing webhooks by id
// require per-device knowledge of current webhook ids, so they stay
// JSON-only — hydrateWebhooks rejects an `update` key with a clear
// message pointing the operator at the JSON editor.
export type WebhookCreateEntry = {
  cid: string; // string for input binding; coerced to number on build
  event: string;
  urls: string; // newline-separated; split on build
  name: string;
  enable: boolean;
};

export type WebhooksState = {
  deleteAll: boolean;
  deleteIds: string; // comma- or whitespace-separated; parsed on build
  creates: WebhookCreateEntry[];
  open: boolean;
};
