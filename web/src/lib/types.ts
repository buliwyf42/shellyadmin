// AltFirmwareVariant is one alternative firmware build a device can run
// (e.g. a Zigbee or Matter variant of the same hardware), from sys.alt.
export interface AltFirmwareVariant {
  id: string;
  name: string;
  desc: string;
  stable?: string;
  beta?: string;
}

// Device is the per-row shape the SPA consumes from /api/devices,
// /api/devices/refresh, /api/devices/refresh-one, and (wrapped in
// DeviceDetail) /api/devices/{target}. v0.3.0 (M8) split the wire
// payload into a slim DeviceListView (the list endpoints) and the full
// Device (only on /api/devices/{target}). The fields below cover the
// slim shape; detail-only fields (batch, fw_id, mqtt_flags_na) are
// optional here because they're only populated by the detail endpoint.
export interface Device {
  mac: string;
  ip: string;
  name: string;
  model: string;
  /** Short app code from Shelly's /shelly endpoint (e.g. "PlugSG3",
   * "Pro4PM"). Friendlier than the canonical SKU `model`; used as the
   * primary "what is this device" label, with `model` demoted to the
   * tooltip. Empty until a scan / refresh has run on v0.1.11+. */
  app?: string;
  /** Production batch from Shelly.GetDeviceInfo. M8: only present on the
   * detail endpoint; the list endpoint drops it. */
  batch?: string;
  /** Long firmware identifier with build hash. M8: only present on the
   * detail endpoint; the list endpoint drops it. */
  fw_id?: string;
  fw: string;
  gen: number;
  online: boolean;
  device_num: number;
  last_seen: string;
  first_seen: string;
  last_refresh_attempt: string;
  last_refresh_ok: boolean;
  last_refresh_error: string;
  mqtt_enabled: boolean | null;
  mqtt_server: string;
  mqtt_client_id: string;
  mqtt_topic_prefix: string;
  /** M8: only present on the detail endpoint; the list endpoint drops it. */
  mqtt_flags_na?: string;
  lat: number | null;
  lon: number | null;
  tz: string;
  sntp_server: string;
  ws_enabled: boolean | null;
  ws_server: string;
  ws_connected: boolean;
  ble_gw_enabled: boolean | null;
  wifi_ssid: string;
  cloud_enabled: boolean | null;
  cloud_connected: boolean;
  matter_enabled: boolean | null;
  eco_mode: boolean | null;
  discoverable: boolean | null;
  auth_required: boolean;
  auth_error: string;
  // Firmware 2.0.0-beta1 additions; older firmware leaves these unset.
  auth_locked_until?: string;
  scheme?: string;
  enhanced_security?: boolean | null;
  tls_cert_valid?: boolean | null;
  tls_allow_insecure?: boolean | null;
  wifi_hostname?: string;
  wifi_channel?: number;
  // Live power telemetry (summed across switch/em/em1/pm1 components).
  // null means the device exposes no power readings; 0 is a real value.
  power_w?: number | null;
  voltage_v?: number | null;
  current_a?: number | null;
  fw_available_stable: string;
  fw_available_beta: string;
  fw_checked_at: string;
  fw_auto_update: string; // "" | "off" | "stable" | "beta"
  /** Per-component instance counts derived from RawStatus on the server.
   * Drive the Devices-page Capabilities column. 0 = device doesn't
   * expose that component type. */
  switch_count?: number;
  cover_count?: number;
  light_count?: number;
  /** Alternative firmware variants (e.g. Zigbee/Matter builds) the device
   * advertises under sys.alt (firmware 2.0.0+). Read-only — switching is not
   * supported by the Shelly API. Absent for devices with no variants. */
  fw_alt?: AltFirmwareVariant[];
  serial: string;
  is_new?: boolean;
  compliant: boolean;
  compliance_issues: string[] | null;
}

export interface ComplianceRules {
  wifi_ssid?: string;
  mqtt_enabled?: boolean | null;
  mqtt_server?: string;
  mqtt_client_id?: string;
  mqtt_topic_prefix?: string;
  mqtt_rpc_ntf?: boolean | null;
  mqtt_status_ntf?: boolean | null;
  mqtt_enable_rpc?: boolean | null;
  mqtt_enable_control?: boolean | null;
  cloud_connected?: boolean | null;
  ws_enabled?: boolean | null;
  ws_connected?: boolean | null;
  ws_server?: string;
  ws_tls_mode?: 'no_validation' | 'default' | 'user' | '';
  ws_ssl_ca?: string;
  ble_gw_enabled?: boolean | null;
  ble_rpc_enable?: boolean | null;
  ble_observer_enable?: boolean | null;
  tz?: string;
  sntp_server?: string;
  lat?: number | null;
  lon?: number | null;
  sys_debug_websocket?: boolean | null;
  sys_debug_udp_host?: string;
  sys_rpc_udp_port?: number | null;
  eco_mode?: boolean | null;
  discoverable?: boolean | null;
  wifi_ap_enabled?: boolean | null;
  wifi_ap_is_open?: boolean | null;
  eth_enabled?: boolean | null;
  eth_ipv4mode?: 'dhcp' | 'static' | '';
  sys_debug_mqtt?: boolean | null;
  matter_enabled?: boolean | null;
  modbus_enabled?: boolean | null;
  zigbee_enabled?: boolean | null;
  // Firmware 2.0.0-beta1 compliance fields:
  enhanced_security?: boolean | null;
  tls_cert_valid?: boolean | null;
  wifi_hostname?: string;
  ble_paired?: boolean | null;
  webhooks_configured?: boolean | null;
  auto_update_stage?: '' | 'off' | 'stable' | 'beta';
  custom_rules?: CustomRule[];
}

export interface CustomRule {
  label: string;
  source: 'device' | 'config' | 'status';
  path: string;
  op: 'eq' | 'ne' | 'contains' | 'regex' | 'exists';
  value: string;
  gen_min: number;
  gen_max: number;
}

export interface AppSettings {
  subnets: string[];
  scan_timeout: number;
  refresh_timeout: number;
  scan_concurrency: number;
  enable_mdns: boolean;
  advanced_mode_enabled: boolean;
  gen2_badge_class?: string;
  gen3_badge_class?: string;
  gen4_badge_class?: string;
  /** Per-device install timeout in seconds. Default 300. */
  firmware_install_timeout?: number;
  /** How often the install_job polls a device's firmware version while waiting for the reboot. Seconds; default 5; bounded [1, 60]. */
  firmware_install_poll_interval?: number;
  /** Scheduled firmware check cadence in seconds. 0 = disabled. */
  firmware_check_interval?: number;
  compliance: ComplianceRules;
  /** Enables the read-only MCP server at next container restart, when no SHELLYADMIN_MCP_TOKEN env var is set. */
  mcp_enabled?: boolean;
  /** Token used to authenticate MCP clients. Stored encrypted at rest. The API GET response substitutes "<set>" if a token is configured (never plaintext). Send "<set>" verbatim to leave the persisted token unchanged on save. */
  mcp_token?: string;
  /** True when SHELLYADMIN_MCP_TOKEN is set in the environment, in which case mcp_enabled / mcp_token in this struct are ignored at boot and the UI should show fields read-only. Read-only — never persisted. */
  mcp_managed_by_env?: boolean;
  /** True when an MCP listener goroutine is currently active. Surfaced in the API GET response so the UI can render a live status badge. Read-only — never persisted. */
  mcp_running?: boolean;
  /** When true AND the operator has an active TOTP enrollment, the login handler refuses password-only auth. Non-enrolled operators can still log in password-only (escape hatch for the first-time enrollment flow). T1 in v0.3.0. */
  totp_required?: boolean;
}

/** GET /api/totp/status response — drives the Settings 2FA card's
 *  Enroll-vs-Disable controls and the "X of N backup codes left" line. */
export interface TOTPStatus {
  enrolled: boolean;
  enrolled_at?: string;
  last_verified_at?: string;
  backup_codes_left?: number;
}

/** POST /api/totp/enroll response — the plaintext secret + 10 recovery
 *  codes surface exactly once on the wire. The SPA flashes them at the
 *  operator and clears them from memory after Verify-Enroll succeeds. */
export interface TOTPEnrollResponse {
  secret: string;
  otpauth_uri: string;
  backup_codes: string[];
  qrcode_format: string;
}

/** Per-token metadata returned by GET /api/tokens. Never carries the
 *  plaintext secret — that's only emitted in the Create response. */
export interface ListedPAT {
  id: string;
  name: string;
  scopes: string[];
  created_at: string;
  last_used_at?: string;
  expires_at?: string;
  revoked_at?: string;
  revoked: boolean;
  expired: boolean;
}

/** GET /api/tokens response. available_scopes is the live catalog so
 *  the create-token form can render the checkbox list without a
 *  separate endpoint. */
export interface ListTokensResponse {
  tokens: ListedPAT[];
  available_scopes: string[];
}

/** POST /api/tokens response. token field surfaces the plaintext
 *  bearer string ONCE — the SPA flashes it to the operator and drops
 *  it from memory after a copy-to-clipboard. */
export interface CreateTokenResponse {
  token: string;
  id: string;
  name: string;
  scopes: string[];
  created_at: string;
  expires_at?: string;
}

export interface FWResult {
  ip: string;
  mac: string;
  current_ver: string;
  stable_ver: string;
  beta_ver: string;
  stable_update: boolean;
  beta_update: boolean;
  status: string; // "ok" | "error" | "na"
  note: string;
  checked_at: string;
}

export interface FirmwareStatus {
  running: boolean;
  done: number;
  total: number;
  results: FWResult[];
}

export interface FirmwareInstallResult {
  ip: string;
  mac: string;
  stage: string;
  from_ver: string;
  to_ver: string;
  status: string; // "pending" | "updating" | "current" | "error" | "unknown" | "skipped"
  detail: string;
}

export interface FirmwareInstallStatus {
  running: boolean;
  done: number;
  total: number;
  results: FirmwareInstallResult[];
}

export interface ScanStatus {
  running: boolean;
  found: number;
  total: number;
  done: number;
  pending: (Device & { is_new: boolean })[];
}

export interface LogEntry {
  id: number;
  ts: string;
  level: string;
  message: string;
  request_id?: string;
  /** Catalog risk level on action-execution rows; empty on every other
   * audit row. Used by the Logs page to render a small badge so
   * high-risk events stand out. */
  risk_level?: string;
}

export interface VersionInfo {
  backend_version: string;
  commit: string;
}

export interface BulkActionTarget {
  mac: string;
  ip: string;
  name: string;
  eligible: boolean;
  reason?: string;
}

export interface BulkActionPreview {
  action: string;
  summary: string;
  warnings: string[];
  targets: BulkActionTarget[];
}

export interface BulkActionResult {
  mac: string;
  ip: string;
  status: string;
  detail: string;
}

/**
 * Mirrors internal/services/device_surface.go:BulkActionRequest. Kept in sync
 * by hand; update when the backend struct gains fields.
 */
export interface BulkActionRequest {
  action: string;
  macs: string[];
  value?: string;
  lat?: number;
  lon?: number;
  enabled?: boolean | null;
  dry_run?: boolean;
}

/**
 * Mirrors internal/core/provisioner/provisioner.go:DeviceInfo. Values may be
 * missing when the device returned only a partial identify response.
 */
export interface ProvisionDeviceInfo {
  name?: string;
  model?: string;
  fw?: string;
  gen?: number;
  ip: string;
}

/** Mirrors internal/core/provisioner/provisioner.go:SectionResult. */
export interface ProvisionSectionResult {
  section: string;
  status: string;
  detail: string;
  restart_required?: boolean;
}

/** Per-IP result returned by POST /api/provision. */
export interface ProvisionResult {
  info: ProvisionDeviceInfo;
  results: ProvisionSectionResult[];
  restart_required?: boolean;
}

/** Mirrors internal/core/firmware/firmware.go:UpdateResult. */
export interface FirmwareUpdateResult {
  ip: string;
  mac: string;
  status: string;
  detail: string;
}

/** Mirrors internal/services/app.go:UploadUserCAResult. */
export interface UploadUserCAResult {
  ip: string;
  status: string;
  chunks: number;
  bytes_sent: number;
  detail: string;
}

export interface DeviceCapability {
  id: string;
  label: string;
  state: string;
  description?: string;
}

export interface DeviceAction {
  id: string;
  label: string;
  description: string;
  risk: string;
  supported: boolean;
  requires_online: boolean;
  reason?: string;
}

export interface DeviceDetail {
  device: Device;
  raw_config: Record<string, unknown>;
  raw_status: Record<string, unknown>;
  capabilities: DeviceCapability[];
  actions: DeviceAction[];
}

export interface DeviceExport {
  version: number;
  exported_at: string;
  device: Device;
  raw_config: Record<string, unknown>;
  raw_status: Record<string, unknown>;
  capabilities: DeviceCapability[];
}

export interface DeviceActionResult {
  action: string;
  status: string;
  detail: string;
  result?: unknown;
}

export interface BackupExport {
  version: number;
  settings: AppSettings;
  templates: Record<string, string>;
  credential_groups?: CredentialGroup[];
  device_group_assignments?: Record<string, string>;
}

export interface ImportReport {
  dry_run: boolean;
  settings_will_apply: boolean;
  templates_create: string[];
  templates_update: string[];
  groups_create: string[];
  groups_update: string[];
  groups_delete: string[];
  assignments_create: number;
  assignments_update: number;
  assignments_delete: number;
}

export interface TemplateRecord {
  name: string;
  content: string;
  credential_ref: string;
}

export interface Credential {
  name: string;
  username: string;
  password: string;
  ha1: string;
  tags: string[];
}

export interface CredentialGroup {
  name: string;
  password: string;
  ha1: string;
  tags: string[];
}
