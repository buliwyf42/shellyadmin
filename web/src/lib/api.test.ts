import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { APIError, api } from './api';

type FetchArgs = { input: RequestInfo | URL; init?: RequestInit };

function jsonResponse(
  body: unknown,
  init: ResponseInit & { headers?: Record<string, string> } = {},
): Response {
  const headers = { 'Content-Type': 'application/json', ...(init.headers ?? {}) };
  return new Response(JSON.stringify(body), { ...init, headers });
}

describe('api client', () => {
  let calls: FetchArgs[] = [];

  beforeEach(() => {
    calls = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push({ input, init });
        const path = typeof input === 'string' ? input : input.toString();
        if (path === '/api/csrf-token') {
          return jsonResponse(
            { csrf_token: 'test-token' },
            { headers: { 'X-CSRF-Token': 'test-token' } },
          );
        }
        if (path === '/api/devices') {
          return jsonResponse([{ mac: 'AA', ip: '10.0.0.1' }]);
        }
        if (path === '/api/scan/start') {
          return jsonResponse({ status: 'started' });
        }
        if (path === '/api/missing') {
          return new Response(JSON.stringify({ error: 'not found' }), {
            status: 404,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (path === '/api/wrong-content') {
          return new Response('<html>oops</html>', {
            status: 200,
            headers: { 'Content-Type': 'text/html' },
          });
        }
        throw new Error(`unexpected fetch: ${path}`);
      }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('GETs without csrf token', async () => {
    const result = await api.getDevices();
    expect(result).toEqual([{ mac: 'AA', ip: '10.0.0.1' }]);
    expect(calls).toHaveLength(1);
    expect(calls[0].init?.method).toBe('GET');
    expect(calls[0].init?.headers).not.toHaveProperty('X-CSRF-Token');
  });

  it('POST fetches csrf-token then includes header', async () => {
    const result = await api.scanStart();
    expect(result).toEqual({ status: 'started' });
    expect(calls).toHaveLength(2);
    expect(calls[0].input).toBe('/api/csrf-token');
    expect(calls[1].input).toBe('/api/scan/start');
    const headers = calls[1].init?.headers as Record<string, string>;
    expect(headers['X-CSRF-Token']).toBe('test-token');
    expect(headers['Content-Type']).toBe('application/json');
  });

  it('throws APIError with status + detail on non-ok responses', async () => {
    await expect(
      () => (api as unknown as { _req: (m: string, p: string) => Promise<unknown> })._req,
    ).toBeDefined();

    // Use a real call that maps to /api/missing via a known endpoint signature.
    // We force it through by mocking deleteCredential against the missing path.
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const path = typeof input === 'string' ? input : input.toString();
        if (path === '/api/csrf-token') {
          return jsonResponse({ csrf_token: 'tok' });
        }
        return new Response(JSON.stringify({ error: 'not found' }), {
          status: 404,
          headers: { 'Content-Type': 'application/json' },
        });
      }),
    );

    try {
      await api.deleteCredential('does-not-exist');
      expect.unreachable('should have thrown');
    } catch (err) {
      expect(err).toBeInstanceOf(APIError);
      const e = err as APIError;
      expect(e.status).toBe(404);
      expect(e.message).toBe('not found');
      expect(e.method).toBe('DELETE');
    }
  });

  it('retries idempotent GET on transient network failure', async () => {
    let attempts = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const path = typeof input === 'string' ? input : input.toString();
        attempts++;
        if (attempts < 3 && path === '/api/devices') {
          throw new TypeError('network blip');
        }
        return jsonResponse([{ mac: 'BB', ip: '10.0.0.2' }]);
      }),
    );
    const result = await api.getDevices();
    expect(result).toEqual([{ mac: 'BB', ip: '10.0.0.2' }]);
    expect(attempts).toBe(3);
  });

  it('does NOT retry mutations on network failure', async () => {
    let attempts = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const path = typeof input === 'string' ? input : input.toString();
        if (path === '/api/csrf-token') {
          return jsonResponse({ csrf_token: 'tok' });
        }
        attempts++;
        throw new TypeError('network down');
      }),
    );
    await expect(api.scanStart()).rejects.toBeInstanceOf(TypeError);
    expect(attempts).toBe(1);
  });

  it('does NOT retry HTTP status errors', async () => {
    let attempts = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        attempts++;
        return new Response(JSON.stringify({ error: 'boom' }), {
          status: 500,
          headers: { 'Content-Type': 'application/json' },
        });
      }),
    );
    await expect(api.getDevices()).rejects.toBeInstanceOf(APIError);
    expect(attempts).toBe(1);
  });

  it('redirects to /login on 401 for non-login paths', async () => {
    const assign = vi.fn();
    const originalLocation = window.location;
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { pathname: '/devices', assign },
    });
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        return new Response(JSON.stringify({ error: 'unauthorized' }), {
          status: 401,
          headers: { 'Content-Type': 'application/json' },
        });
      }),
    );
    try {
      await expect(api.getDevices()).rejects.toBeInstanceOf(APIError);
      expect(assign).toHaveBeenCalledWith('/login');
    } finally {
      Object.defineProperty(window, 'location', {
        configurable: true,
        value: originalLocation,
      });
    }
  });

  it('does NOT redirect on 401 when already on /login', async () => {
    const assign = vi.fn();
    const originalLocation = window.location;
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { pathname: '/login', assign },
    });
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const path = typeof input === 'string' ? input : input.toString();
        if (path === '/api/csrf-token') return jsonResponse({ csrf_token: 'tok' });
        return new Response(JSON.stringify({ error: 'bad credentials' }), {
          status: 401,
          headers: { 'Content-Type': 'application/json' },
        });
      }),
    );
    try {
      await expect(api.login('u', 'p')).rejects.toBeInstanceOf(APIError);
      expect(assign).not.toHaveBeenCalled();
    } finally {
      Object.defineProperty(window, 'location', {
        configurable: true,
        value: originalLocation,
      });
    }
  });

  it('throws APIError when response is not JSON', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        return new Response('<html></html>', {
          status: 200,
          headers: { 'Content-Type': 'text/html' },
        });
      }),
    );
    try {
      await api.getDevices();
      expect.unreachable('should have thrown');
    } catch (err) {
      expect(err).toBeInstanceOf(APIError);
      const e = err as APIError;
      expect(e.message).toMatch(/expected JSON/i);
    }
  });

  // S16 from the consolidated review — lock in the high-blast-radius
  // mutation paths (bulk reboot, firmware update, provision). Each
  // exercises the CSRF + body-shape contract that the corresponding
  // page (Devices.svelte / Firmware.svelte / Provision.svelte) relies
  // on. A regression that breaks the request body silently here would
  // show up in production as "reboot did nothing".

  it('bulk reboot sends action+macs body with CSRF header', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push({ input, init });
        const path = typeof input === 'string' ? input : input.toString();
        if (path === '/api/csrf-token') {
          return jsonResponse({ csrf_token: 'tok' });
        }
        if (path === '/api/bulk') {
          return jsonResponse({
            dry_run: false,
            results: [{ mac: 'AA:BB:CC:DD:EE:01', status: 'ok' }],
          });
        }
        throw new Error('unexpected: ' + path);
      }),
    );
    const res = await api.bulk({ action: 'reboot', macs: ['AA:BB:CC:DD:EE:01'] });
    expect(res.results[0].status).toBe('ok');
    const bulk = calls.find((c) => (typeof c.input === 'string' ? c.input : '') === '/api/bulk');
    expect(bulk).toBeDefined();
    const body = JSON.parse(bulk!.init!.body as string);
    expect(body.action).toBe('reboot');
    expect(body.macs).toEqual(['AA:BB:CC:DD:EE:01']);
    expect(body.dry_run).toBe(false);
    const headers = bulk!.init!.headers as Record<string, string>;
    expect(headers['X-CSRF-Token']).toBe('tok');
  });

  it('firmwareUpdate carries stage in body, not query', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push({ input, init });
        const path = typeof input === 'string' ? input : input.toString();
        if (path === '/api/csrf-token') {
          return jsonResponse({ csrf_token: 'tok' });
        }
        if (path === '/api/firmware/update') {
          return jsonResponse({ status: 'started', job_id: 42, total: 1 });
        }
        throw new Error('unexpected: ' + path);
      }),
    );
    const res = await api.firmwareUpdate(['AA'], 'beta');
    expect(res.job_id).toBe(42);
    const fw = calls.find(
      (c) => (typeof c.input === 'string' ? c.input : '') === '/api/firmware/update',
    );
    const body = JSON.parse(fw!.init!.body as string);
    expect(body.stage).toBe('beta');
    expect(body.macs).toEqual(['AA']);
  });

  it('previewBulk forces dry_run=true regardless of caller', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push({ input, init });
        const path = typeof input === 'string' ? input : input.toString();
        if (path === '/api/csrf-token') return jsonResponse({ csrf_token: 'tok' });
        if (path === '/api/bulk')
          return jsonResponse({ dry_run: true, preview: { eligible: [], skipped: [] } });
        throw new Error('unexpected: ' + path);
      }),
    );
    await api.previewBulk({ action: 'reboot', macs: ['AA'] });
    const bulk = calls.find((c) => (typeof c.input === 'string' ? c.input : '') === '/api/bulk');
    const body = JSON.parse(bulk!.init!.body as string);
    expect(body.dry_run).toBe(true);
  });
});
