import { describe, expect, it } from 'vitest';
import {
  compareDevices,
  formatCoords,
  generationLabel,
  refreshState,
  refreshStateBadgeClass,
  refreshStateText,
  refreshStateTitle,
  statusBadgeClass,
  statusText,
  supportClass,
  supportTitle,
} from './deviceFormatters';
import type { Device } from './types';

function dev(overrides: Partial<Device> = {}): Device {
  return {
    mac: 'AA:BB:CC:DD:EE:01',
    ip: '192.168.1.10',
    name: 'test-device',
    model: 'SNSW-001P16EU',
    fw: '2.0.0',
    gen: 3,
    online: true,
    device_num: 1,
    last_seen: '2026-05-11T14:00:00Z',
    first_seen: '2026-01-01T00:00:00Z',
    last_refresh_attempt: '2026-05-11T14:00:05Z',
    last_refresh_ok: true,
    last_refresh_error: '',
    mqtt_enabled: false,
    mqtt_server: '',
    mqtt_client_id: '',
    mqtt_topic_prefix: '',
    lat: null,
    lon: null,
    tz: '',
    sntp_server: '',
    ws_enabled: false,
    ws_server: '',
    ws_connected: false,
    ble_gw_enabled: false,
    wifi_ssid: '',
    cloud_enabled: false,
    cloud_connected: false,
    matter_enabled: false,
    eco_mode: false,
    discoverable: false,
    auth_required: false,
    auth_error: '',
    fw_available_stable: '',
    fw_available_beta: '',
    fw_checked_at: '',
    fw_auto_update: '',
    serial: 'S001',
    compliant: true,
    compliance_issues: [],
    ...overrides,
  };
}

describe('statusBadgeClass', () => {
  it('returns "bg-secondary" for null + undefined', () => {
    expect(statusBadgeClass(null)).toBe('bg-secondary');
    expect(statusBadgeClass(undefined)).toBe('bg-secondary');
  });
  it('returns the positive class for true', () => {
    expect(statusBadgeClass(true)).toBe('bg-success');
    expect(statusBadgeClass(true, 'bg-info')).toBe('bg-info');
  });
  it('returns the negative class for false', () => {
    expect(statusBadgeClass(false)).toBe('bg-danger');
    expect(statusBadgeClass(false, 'bg-success', 'bg-warning')).toBe('bg-warning');
  });
});

describe('statusText', () => {
  it('returns "n/a" for null + undefined', () => {
    expect(statusText(null)).toBe('n/a');
    expect(statusText(undefined)).toBe('n/a');
  });
  it('returns custom labels when provided', () => {
    expect(statusText(true, 'Yes', 'No')).toBe('Yes');
    expect(statusText(false, 'Yes', 'No')).toBe('No');
  });
});

describe('formatCoords', () => {
  it('returns "n/a" when either coordinate is null', () => {
    expect(formatCoords(dev({ lat: null, lon: 13.4 }))).toBe('n/a');
    expect(formatCoords(dev({ lat: 52.5, lon: null }))).toBe('n/a');
  });
  it('formats both with 5 decimal places', () => {
    expect(formatCoords(dev({ lat: 52.5, lon: 13.40666 }))).toBe('52.50000, 13.40666');
  });
});

describe('refreshState helpers', () => {
  it('reports "fresh" when last_refresh_ok is true', () => {
    const d = dev({ last_refresh_ok: true });
    expect(refreshState(d)).toBe('fresh');
    expect(refreshStateBadgeClass(d)).toBe('bg-success');
    expect(refreshStateText(d)).toBe('Fresh');
  });
  it('reports "stale" when last_refresh_ok is false', () => {
    const d = dev({ last_refresh_ok: false });
    expect(refreshState(d)).toBe('stale');
    expect(refreshStateBadgeClass(d)).toBe('bg-secondary');
    expect(refreshStateText(d)).toBe('Stale');
  });
  it('refreshStateTitle includes the failure reason when stale', () => {
    const title = refreshStateTitle(
      dev({
        last_refresh_ok: false,
        last_refresh_error: 'auth required',
        last_refresh_attempt: '2026-05-11T14:00:05Z',
        last_seen: '2026-05-10T10:00:00Z',
      }),
    );
    expect(title).toContain('auth required');
    expect(title).toContain('Last attempt');
  });
});

describe('compareDevices', () => {
  it('sorts by device_num numerically', () => {
    const a = dev({ device_num: 5 });
    const b = dev({ device_num: 10 });
    expect(compareDevices(a, b, 'device_num')).toBeLessThan(0);
    expect(compareDevices(b, a, 'device_num')).toBeGreaterThan(0);
  });
  it('sorts by ip with numeric comparison so 192.168.1.10 > 192.168.1.2', () => {
    const a = dev({ ip: '192.168.1.10' });
    const b = dev({ ip: '192.168.1.2' });
    expect(compareDevices(a, b, 'ip')).toBeGreaterThan(0);
  });
  it('sorts by model on the displayed (app first) text', () => {
    const a = dev({ app: 'PlugSG3', model: 'SNSW-001P16EU' });
    const b = dev({ app: 'Pro4PM', model: 'SNSW-104PMv2' });
    // PlugSG3 < Pro4PM alphabetically → a comes first
    expect(compareDevices(a, b, 'model')).toBeLessThan(0);
  });
  it('sorts unknown power as -1 (devices without telemetry come first)', () => {
    const a = dev({ power_w: undefined });
    const b = dev({ power_w: 0 });
    expect(compareDevices(a, b, 'power_w')).toBeLessThan(0);
  });
  it('returns 0 for unknown sort keys', () => {
    expect(compareDevices(dev(), dev(), 'no-such-column')).toBe(0);
  });
});

describe('generation badge — feature-frozen override', () => {
  it('labels an ordinary Gen 2 device without the frozen marker', () => {
    const d = dev({ gen: 2, fw_frozen: false });
    expect(generationLabel(d)).toBe('Gen 2.x');
  });

  it('labels a feature-frozen device regardless of its gen', () => {
    const d = dev({ gen: 2, fw_frozen: true });
    expect(generationLabel(d)).toBe('Gen 2.x (frozen)');
  });

  it('uses the configurable gen_frozen_badge_class override, not the plain gen color', () => {
    const d = dev({ gen: 2, fw_frozen: true });
    const settings = { gen2_badge_class: 'bg-danger', gen_frozen_badge_class: 'bg-secondary' };
    expect(supportClass(d, settings as never)).toBe('bg-secondary');
  });

  it('falls back to the default frozen color when unset', () => {
    const d = dev({ gen: 2, fw_frozen: true });
    expect(supportClass(d, null)).toBe('bg-warning text-dark');
  });

  it('gives the frozen tooltip precedence over the plain gen tooltip', () => {
    const d = dev({ gen: 3, fw_frozen: true });
    expect(supportTitle(d)).toContain('Firmware Update Policy');
  });
});
