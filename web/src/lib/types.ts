export interface Device {
  mac: string
  ip: string
  name: string
  model: string
  fw: string
  gen: number
  online: boolean
  device_num: number
  mqtt_enabled: boolean | null
  mqtt_server: string
  mqtt_client_id: string
  mqtt_topic_prefix: string
  mqtt_flags_na: string
  lat: number | null
  lon: number | null
  tz: string
  time_format: string
  sntp_server: string
  ws_enabled: boolean | null
  ws_server: string
  ws_connected: boolean
  ble_gw_enabled: boolean | null
  wifi_ssid: string
  cloud_enabled: boolean | null
  cloud_connected: boolean
  matter_enabled: boolean | null
  eco_mode: boolean | null
  discoverable: boolean | null
  fw_status: string
  fw_available_ver: string
  serial: string
  is_new?: boolean
  compliant: boolean
  compliance_issues: string[] | null
}

export interface ComplianceRules {
  wifi_ssid?: string
  mqtt_enabled?: boolean | null
  mqtt_server?: string
  mqtt_client_id?: string
  mqtt_topic_prefix?: string
  mqtt_rpc_ntf?: boolean | null
  mqtt_status_ntf?: boolean | null
  mqtt_enable_rpc?: boolean | null
  mqtt_enable_control?: boolean | null
  cloud_connected?: boolean | null
  ws_enabled?: boolean | null
  ws_connected?: boolean | null
  ws_server?: string
  ws_ssl_ca?: string
  ble_gw_enabled?: boolean | null
  ble_rpc_enable?: boolean | null
  tz?: string
  sntp_server?: string
  lat?: number | null
  lon?: number | null
  time_format?: string
  eco_mode?: boolean | null
  discoverable?: boolean | null
  custom_rules?: CustomRule[]
}

export interface CustomRule {
  label: string
  source: 'device' | 'config' | 'status'
  path: string
  op: 'eq' | 'ne' | 'contains' | 'regex' | 'exists'
  value: string
  gen_min: number
  gen_max: number
}

export interface AppSettings {
  subnets: string[]
  scan_timeout: number
  scan_concurrency: number
  compliance: ComplianceRules
}

export interface FWResult {
  ip: string
  mac: string
  current_ver: string
  available_ver: string
  update_available: boolean
  status: string
  note: string
  stage: string
}

export interface FirmwareStatus {
  running: boolean
  done: number
  total: number
  results: FWResult[]
}

export interface ScanStatus {
  running: boolean
  found: number
  total: number
  done: number
  pending: (Device & { is_new: boolean })[]
}

export interface LogEntry {
  id: number
  ts: string
  level: string
  message: string
}

export interface DebugLogResponse {
  lines: string[]
}
