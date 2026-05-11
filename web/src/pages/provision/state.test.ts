import { describe, expect, it } from 'vitest';
import {
  buildCover,
  buildSys,
  buildWebhooks,
  buildZigbeeOps,
  createCoverState,
  createSysState,
  createWebhooksState,
  createZigbeeOpsState,
  hydrateCover,
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

describe('buildCover', () => {
  it('returns null when no fields are toggled (id alone is not enough)', () => {
    expect(buildCover(createCoverState())).toBeNull();
  });

  it('always emits id when any field is toggled', () => {
    const s = createCoverState();
    s.id = 1;
    s.nameEnabled = true;
    s.name = 'kitchen-blind';
    expect(buildCover(s)).toEqual({ id: 1, name: 'kitchen-blind' });
  });

  it('emits the slat sub-object only when slatEnabled and at least one slat field is set', () => {
    const s = createCoverState();
    s.slatEnabled = true;
    // slat itself toggled on, but no sub-fields set -> slat is null, so cover returns null
    expect(buildCover(s)).toBeNull();

    s.slat.enableField = true;
    s.slat.enable = true;
    s.slat.openTimeEnabled = true;
    s.slat.openTime = 2.0;
    expect(buildCover(s)).toEqual({
      id: 0,
      slat: { enable: true, open_time: 2.0 },
    });
  });

  it('emits maxtime_open/close numbers', () => {
    const s = createCoverState();
    s.maxtimeOpenEnabled = true;
    s.maxtimeOpen = 25;
    s.maxtimeCloseEnabled = true;
    s.maxtimeClose = 30;
    expect(buildCover(s)).toEqual({ id: 0, maxtime_open: 25, maxtime_close: 30 });
  });
});

describe('hydrateCover', () => {
  it('rejects unsupported advanced fields with a JSON-editor pointer', () => {
    const r = hydrateCover({ id: 0, obstruction_detection: { enable: true } });
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.reason).toMatch(/switch to JSON view/i);
  });

  it('round-trips slat sub-object', () => {
    const r = hydrateCover({
      id: 1,
      name: 'a',
      maxtime_open: 30,
      slat: {
        enable: true,
        open_time: 1.5,
        close_time: 1.5,
        precise_ctl: true,
        step_pos: 10,
      },
    });
    expect(r.ok).toBe(true);
    if (r.ok) {
      expect(r.state.id).toBe(1);
      expect(r.state.nameEnabled).toBe(true);
      expect(r.state.name).toBe('a');
      expect(r.state.slatEnabled).toBe(true);
      expect(r.state.slat.enableField).toBe(true);
      expect(r.state.slat.enable).toBe(true);
      expect(r.state.slat.openTime).toBe(1.5);
      expect(buildCover(r.state)).toEqual({
        id: 1,
        name: 'a',
        maxtime_open: 30,
        slat: {
          enable: true,
          open_time: 1.5,
          close_time: 1.5,
          precise_ctl: true,
          step_pos: 10,
        },
      });
    }
  });

  it('rejects unknown slat sub-keys', () => {
    const r = hydrateCover({ id: 0, slat: { mystery_field: true } });
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.reason).toMatch(/slat contains unsupported key/i);
  });

  it('rejects non-integer id', () => {
    const r = hydrateCover({ id: 1.5 });
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.reason).toMatch(/non-negative integer/i);
  });
});

describe('buildZigbeeOps', () => {
  it('returns null when no operations are enabled', () => {
    expect(buildZigbeeOps(createZigbeeOpsState())).toBeNull();
  });

  it('returns null when an operation is enabled but eui64 is empty', () => {
    const s = createZigbeeOpsState();
    s.sendCommandEnabled = true;
    expect(buildZigbeeOps(s)).toBeNull();
  });

  it('builds Zigbee.SendCommand with optional payload', () => {
    const s = createZigbeeOpsState();
    s.sendCommandEnabled = true;
    s.sendCommand = {
      eui64: '0x00158d0001abcd1234',
      ep: 1,
      cluster: 6,
      cmd: 1,
      payload: '0102',
    };
    expect(buildZigbeeOps(s)).toEqual({
      'Zigbee.SendCommand': {
        eui64: '0x00158d0001abcd1234',
        ep: 1,
        cluster: 6,
        cmd: 1,
        payload: '0102',
      },
    });
  });

  it('omits Zigbee.SendCommand payload when blank', () => {
    const s = createZigbeeOpsState();
    s.sendCommandEnabled = true;
    s.sendCommand = { eui64: '0xAA', ep: 1, cluster: 0, cmd: 0, payload: '   ' };
    expect(buildZigbeeOps(s)).toEqual({
      'Zigbee.SendCommand': { eui64: '0xAA', ep: 1, cluster: 0, cmd: 0 },
    });
  });

  it('parses ReadAttr attrs from comma- or whitespace-separated input', () => {
    const s = createZigbeeOpsState();
    s.readAttrEnabled = true;
    s.readAttr = { eui64: '0xBB', ep: 1, cluster: 4, attrs: ' 0, 4 5,, 1024' };
    expect(buildZigbeeOps(s)).toEqual({
      'Zigbee.ReadAttr': { eui64: '0xBB', ep: 1, cluster: 4, attrs: [0, 4, 5, 1024] },
    });
  });

  it('drops ReadAttr when no valid attribute ids parsed', () => {
    const s = createZigbeeOpsState();
    s.readAttrEnabled = true;
    s.readAttr = { eui64: '0xBB', ep: 1, cluster: 0, attrs: 'abc, -1' };
    expect(buildZigbeeOps(s)).toBeNull();
  });

  it('builds WriteAttr from valid attrs JSON array', () => {
    const s = createZigbeeOpsState();
    s.writeAttrEnabled = true;
    s.writeAttr = {
      eui64: '0xCC',
      ep: 1,
      cluster: 6,
      attrsJSON: '[{"id":0,"type":"uint8","value":1}]',
    };
    expect(buildZigbeeOps(s)).toEqual({
      'Zigbee.WriteAttr': {
        eui64: '0xCC',
        ep: 1,
        cluster: 6,
        attrs: [{ id: 0, type: 'uint8', value: 1 }],
      },
    });
  });

  it('drops WriteAttr when attrs JSON is invalid or non-array', () => {
    const s = createZigbeeOpsState();
    s.writeAttrEnabled = true;
    s.writeAttr = { eui64: '0xCC', ep: 1, cluster: 0, attrsJSON: 'not-json' };
    expect(buildZigbeeOps(s)).toBeNull();

    s.writeAttr.attrsJSON = '{"not":"array"}';
    expect(buildZigbeeOps(s)).toBeNull();

    s.writeAttr.attrsJSON = '[]';
    expect(buildZigbeeOps(s)).toBeNull();
  });

  it('combines multiple ops in one gen2_rpc-shaped output', () => {
    const s = createZigbeeOpsState();
    s.sendCommandEnabled = true;
    s.sendCommand = { eui64: '0xAA', ep: 1, cluster: 0, cmd: 0, payload: '' };
    s.readAttrEnabled = true;
    s.readAttr = { eui64: '0xAA', ep: 1, cluster: 0, attrs: '0' };
    const out = buildZigbeeOps(s) as Record<string, unknown>;
    expect(Object.keys(out).sort()).toEqual(['Zigbee.ReadAttr', 'Zigbee.SendCommand']);
  });
});
