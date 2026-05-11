// Pure presentation helpers used by Devices.svelte + the children it
// spawns (DeviceTable, DeviceRowActions, ColumnPicker). Extracted from
// Devices.svelte in v0.3.0 (M2 — docs/plans/phase-4b-refactor-block.md
// Block 4b.3) so the Svelte side stays under 500 LOC each, the helpers
// get vitest coverage in deviceFormatters.test.ts, and future device-
// table refinements don't keep ballooning the parent component.

import { genBadgeClass, genLabel, genTitle } from './genBadge';
import { formatDateTime } from './time';
import type { AppSettings, Device } from './types';

/** Friendly generation label ("Gen 2", "Gen 3", "Gen 4"). */
export function generationLabel(device: Device): string {
  return genLabel(device.gen);
}

/** Bootstrap badge class for the device-generation cell. AppSettings
 * may down-tier old gens via deprecation flags; pass null when settings
 * haven't loaded yet. */
export function supportClass(device: Device, appSettings: AppSettings | null): string {
  return genBadgeClass(device.gen, appSettings);
}

/** Tooltip text describing the generation tier. */
export function supportTitle(device: Device): string {
  return genTitle(device.gen);
}

/** Three-state badge class for boolean device fields (true/false/null).
 * Pass positive/negative/unknown overrides to customise colours per
 * column (e.g. cloud_connected uses bg-info for "on"). */
export function statusBadgeClass(
  value: boolean | null | undefined,
  positive = 'bg-success',
  negative = 'bg-danger',
  unknown = 'bg-secondary',
): string {
  if (value === null || value === undefined) return unknown;
  return value ? positive : negative;
}

/** Three-state text label for boolean device fields. */
export function statusText(
  value: boolean | null | undefined,
  on = 'On',
  off = 'Off',
  na = 'n/a',
): string {
  if (value === null || value === undefined) return na;
  return value ? on : off;
}

/** Currently always false — placeholder for "device's MQTT is overridden
 * by Shelly Cloud management". Kept so the column can flip on later
 * without touching every callsite. */
export function mqttManagedByCloud(_device: Device): boolean {
  return false;
}

/** Formatted lat,lon pair or "n/a" when either is null. */
export function formatCoords(device: Device): string {
  if (device.lat === null || device.lon === null) return 'n/a';
  return `${device.lat.toFixed(5)}, ${device.lon.toFixed(5)}`;
}

/** Currently always true — placeholder for future per-device websocket
 * capability gating. */
export function supportsWebSocket(_device: Device): boolean {
  return true;
}

/** "fresh" when the last refresh probe returned data, else "stale". */
export function refreshState(device: Device): 'fresh' | 'stale' {
  return device.last_refresh_ok ? 'fresh' : 'stale';
}

/** Badge class for the refresh-state column. */
export function refreshStateBadgeClass(device: Device): string {
  return refreshState(device) === 'fresh' ? 'bg-success' : 'bg-secondary';
}

/** Badge text for the refresh-state column. */
export function refreshStateText(device: Device): string {
  return refreshState(device) === 'fresh' ? 'Fresh' : 'Stale';
}

/** Tooltip text for the refresh-state badge: when the last successful
 * refresh was, plus the failure reason when applicable. */
export function refreshStateTitle(device: Device): string {
  if (device.last_refresh_ok) {
    return `Last successful refresh: ${formatDateTime(device.last_seen)}`;
  }
  const lastSuccess = device.last_seen ? formatDateTime(device.last_seen) : 'never';
  const lastAttempt = device.last_refresh_attempt
    ? formatDateTime(device.last_refresh_attempt)
    : 'unknown';
  const reason = device.last_refresh_error || 'latest refresh did not return device data';
  return `Latest refresh failed: ${reason}. Last attempt: ${lastAttempt}. Last successful refresh: ${lastSuccess}.`;
}

/** Sort-comparator used by the Devices table header click handlers. The
 * `key` argument matches the column.key strings declared in
 * stores.ts:deviceColumns. Unknown keys collapse to 0 (no-op). */
export function compareDevices(a: Device, b: Device, key: string): number {
  switch (key) {
    case 'device_num':
      return a.device_num - b.device_num;
    case 'name':
      return (a.name || a.serial || a.mac).localeCompare(b.name || b.serial || b.mac);
    case 'ip':
      return a.ip.localeCompare(b.ip, undefined, { numeric: true });
    case 'mac':
      return a.mac.localeCompare(b.mac);
    case 'gen':
      return a.gen - b.gen;
    case 'model':
      // Sort on the displayed text (app code first, model SKU fallback)
      // so the column header click matches what the eye sees in the
      // cells.
      return (a.app || a.model || '').localeCompare(b.app || b.model || '');
    case 'fw':
      return (a.fw || '').localeCompare(b.fw || '');
    case 'online':
      return Number(a.online) - Number(b.online);
    case 'wifi_ssid':
      return (a.wifi_ssid || '').localeCompare(b.wifi_ssid || '');
    case 'mqtt_enabled':
      return Number(Boolean(a.mqtt_enabled)) - Number(Boolean(b.mqtt_enabled));
    case 'mqtt_server':
      return (a.mqtt_server || '').localeCompare(b.mqtt_server || '');
    case 'mqtt_client_id':
      return (a.mqtt_client_id || '').localeCompare(b.mqtt_client_id || '');
    case 'mqtt_topic_prefix':
      return (a.mqtt_topic_prefix || '').localeCompare(b.mqtt_topic_prefix || '');
    case 'cloud_connected':
      return Number(a.cloud_connected) - Number(b.cloud_connected);
    case 'ws_connected':
      return Number(a.ws_connected) - Number(b.ws_connected);
    case 'tz':
      return (a.tz || '').localeCompare(b.tz || '');
    case 'sntp_server':
      return (a.sntp_server || '').localeCompare(b.sntp_server || '');
    case 'serial':
      return (a.serial || '').localeCompare(b.serial || '');
    case 'matter_enabled':
      return Number(Boolean(a.matter_enabled)) - Number(Boolean(b.matter_enabled));
    case 'ble_gw_enabled':
      return Number(Boolean(a.ble_gw_enabled)) - Number(Boolean(b.ble_gw_enabled));
    case 'coords':
      return formatCoords(a).localeCompare(formatCoords(b));
    case 'eco_mode':
      return Number(Boolean(a.eco_mode)) - Number(Boolean(b.eco_mode));
    case 'discoverable':
      return Number(Boolean(a.discoverable)) - Number(Boolean(b.discoverable));
    case 'scheme':
      return (a.scheme || '').localeCompare(b.scheme || '');
    case 'wifi_hostname':
      return (a.wifi_hostname || '').localeCompare(b.wifi_hostname || '');
    case 'wifi_channel':
      return (a.wifi_channel || 0) - (b.wifi_channel || 0);
    case 'enhanced_security':
      return Number(Boolean(a.enhanced_security)) - Number(Boolean(b.enhanced_security));
    case 'tls_cert_valid':
      return Number(Boolean(a.tls_cert_valid)) - Number(Boolean(b.tls_cert_valid));
    case 'power_w':
      return (a.power_w ?? -1) - (b.power_w ?? -1);
    case 'voltage_v':
      return (a.voltage_v ?? -1) - (b.voltage_v ?? -1);
    case 'current_a':
      return (a.current_a ?? -1) - (b.current_a ?? -1);
    case 'first_seen':
      return (a.first_seen || '').localeCompare(b.first_seen || '');
    case 'last_seen':
      return (a.last_seen || '').localeCompare(b.last_seen || '');
    case 'compliance':
      return Number(a.compliant) - Number(b.compliant);
    default:
      return 0;
  }
}
