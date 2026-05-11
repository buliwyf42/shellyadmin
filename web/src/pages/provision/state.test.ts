import { describe, expect, it } from 'vitest';
import {
  buildSys,
  buildWebhooks,
  createSysState,
  createWebhooksState,
  hydrateSys,
  hydrateWebhooks,
  isTLSServerURL,
} from './state';

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

describe('buildWebhooks', () => {
  it('returns null when form is empty', () => {
    expect(buildWebhooks(createWebhooksState())).toBeNull();
  });

  it('emits delete_all only when toggled', () => {
    const s = createWebhooksState();
    s.deleteAll = true;
    expect(buildWebhooks(s)).toEqual({ delete_all: true });
  });

  it('parses delete ids from comma- or whitespace-separated input', () => {
    const s = createWebhooksState();
    s.deleteIds = ' 3, 7  12,, 5';
    expect(buildWebhooks(s)).toEqual({ delete: [3, 7, 12, 5] });
  });

  it('drops non-positive-int delete ids silently', () => {
    const s = createWebhooksState();
    s.deleteIds = '3, abc, -1, 0, 12';
    expect(buildWebhooks(s)).toEqual({ delete: [3, 12] });
  });

  it('builds a create entry with multi-line URLs', () => {
    const s = createWebhooksState();
    s.creates = [
      {
        cid: '0',
        event: 'input.toggle_on',
        urls: 'https://a.example/hook\nhttps://b.example/hook\n',
        name: 'my hook',
        enable: true,
      },
    ];
    expect(buildWebhooks(s)).toEqual({
      create: [
        {
          cid: 0,
          event: 'input.toggle_on',
          urls: ['https://a.example/hook', 'https://b.example/hook'],
          name: 'my hook',
        },
      ],
    });
  });

  it('emits enable=false only when explicitly disabled (default-true semantics)', () => {
    const s = createWebhooksState();
    s.creates = [{ cid: '1', event: 'switch.on', urls: 'https://x/y', name: '', enable: false }];
    const out = buildWebhooks(s) as { create: Record<string, unknown>[] };
    expect(out.create[0]).toEqual({
      cid: 1,
      event: 'switch.on',
      urls: ['https://x/y'],
      enable: false,
    });
  });

  it('skips create entries missing event or URLs', () => {
    const s = createWebhooksState();
    s.creates = [
      { cid: '0', event: '', urls: 'https://x', name: '', enable: true },
      { cid: '0', event: 'x', urls: '', name: '', enable: true },
      { cid: '0', event: 'good', urls: 'https://ok', name: '', enable: true },
    ];
    const out = buildWebhooks(s) as { create: Record<string, unknown>[] };
    expect(out.create).toHaveLength(1);
    expect(out.create[0]).toMatchObject({ event: 'good' });
  });
});

describe('hydrateWebhooks', () => {
  it('rejects an `update` block with a JSON-editor pointer', () => {
    const r = hydrateWebhooks({ update: [{ id: 1, name: 'x' }] });
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.reason).toMatch(/JSON-only/i);
  });

  it('rejects unsupported top-level keys', () => {
    const r = hydrateWebhooks({ something_else: true });
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.reason).toMatch(/unsupported key/i);
  });

  it('round-trips delete_all + delete + create', () => {
    const template = {
      delete_all: true,
      delete: [3, 7],
      create: [
        {
          cid: 0,
          event: 'input.toggle_on',
          urls: ['https://a.example/hook'],
          name: 'a',
        },
      ],
    };
    const r = hydrateWebhooks(template);
    expect(r.ok).toBe(true);
    if (r.ok) {
      expect(r.state.deleteAll).toBe(true);
      expect(r.state.deleteIds).toBe('3, 7');
      expect(r.state.creates).toHaveLength(1);
      expect(r.state.creates[0]).toMatchObject({
        cid: '0',
        event: 'input.toggle_on',
        urls: 'https://a.example/hook',
        name: 'a',
        enable: true,
      });
      // Build back to JSON and confirm shape (sans optional fields that
      // default-true webhooks don't emit).
      expect(buildWebhooks(r.state)).toEqual({
        delete_all: true,
        delete: [3, 7],
        create: [
          {
            cid: 0,
            event: 'input.toggle_on',
            urls: ['https://a.example/hook'],
            name: 'a',
          },
        ],
      });
    }
  });

  it('rejects non-integer delete ids', () => {
    const r = hydrateWebhooks({ delete: [3, 'oops'] });
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.reason).toMatch(/positive integer/i);
  });

  it('rejects create entries missing cid', () => {
    const r = hydrateWebhooks({ create: [{ event: 'x', urls: ['https://y'] }] });
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.reason).toMatch(/cid/i);
  });
});
