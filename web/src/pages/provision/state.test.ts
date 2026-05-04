import { describe, expect, it } from 'vitest';
import { buildSys, createSysState, hydrateSys, isTLSServerURL } from './state';

describe('isTLSServerURL', () => {
  it('identifies wss:// as TLS', () => {
    expect(isTLSServerURL('wss://mqtt.example.com')).toBe(true);
    expect(isTLSServerURL('  WSS://mqtt.example.com')).toBe(true);
  });

  it('identifies plain ws:// as non-TLS', () => {
    expect(isTLSServerURL('ws://mqtt.example.com')).toBe(false);
  });

  it('handles empty strings', () => {
    expect(isTLSServerURL('')).toBe(false);
    expect(isTLSServerURL('   ')).toBe(false);
  });
});

describe('buildSys', () => {
  it('returns null when section disabled', () => {
    const state = createSysState();
    expect(buildSys(state)).toBeNull();
  });

  it('returns null when enabled but no fields toggled', () => {
    const state = createSysState();
    expect(buildSys(state)).toBeNull();
  });

  it('builds device.name when nameEnabled', () => {
    const state = createSysState();
    state.nameEnabled = true;
    state.name = 'shelly-{device_name}';
    const built = buildSys(state);
    expect(built).toEqual({ device: { name: 'shelly-{device_name}' } });
  });

  it('builds sntp.server when sntpEnabled', () => {
    const state = createSysState();
    state.sntpEnabled = true;
    state.sntp = 'time.cloudflare.com';
    expect(buildSys(state)).toEqual({ sntp: { server: 'time.cloudflare.com' } });
  });

  it('builds nested debug.websocket.enable when debugWSEnabled', () => {
    const state = createSysState();
    state.debugWSEnabled = true;
    state.debugWS = true;
    expect(buildSys(state)).toEqual({ debug: { websocket: { enable: true } } });
  });
});

describe('hydrateSys', () => {
  it('rejects unknown top-level fields', () => {
    const result = hydrateSys({ name: 'x', evil_unknown_field: 1 });
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.reason).toMatch(/cannot represent/i);
  });

  it('accepts device.name and enables nameEnabled', () => {
    const result = hydrateSys({ device: { name: 'living-room' } });
    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.state.nameEnabled).toBe(true);
      expect(result.state.name).toBe('living-room');
    }
  });

  it('round-trips sntp.server', () => {
    const result = hydrateSys({ sntp: { server: 'ntp.local' } });
    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.state.sntpEnabled).toBe(true);
      expect(result.state.sntp).toBe('ntp.local');
      // Build back to JSON, and confirm we get the same shape.
      expect(buildSys(result.state)).toEqual({ sntp: { server: 'ntp.local' } });
    }
  });

  it('rejects disagreeing top vs nested name fields', () => {
    const result = hydrateSys({ name: 'a', device: { name: 'b' } });
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.reason).toMatch(/disagree/i);
  });
});
