import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError, api } from './api'

type FetchArgs = { input: RequestInfo | URL; init?: RequestInit }

function jsonResponse(body: unknown, init: ResponseInit & { headers?: Record<string, string> } = {}): Response {
  const headers = { 'Content-Type': 'application/json', ...(init.headers ?? {}) }
  return new Response(JSON.stringify(body), { ...init, headers })
}

describe('api client', () => {
  let calls: FetchArgs[] = []

  beforeEach(() => {
    calls = []
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ input, init })
      const path = typeof input === 'string' ? input : input.toString()
      if (path === '/api/csrf-token') {
        return jsonResponse({ csrf_token: 'test-token' }, { headers: { 'X-CSRF-Token': 'test-token' } })
      }
      if (path === '/api/devices') {
        return jsonResponse([{ mac: 'AA', ip: '10.0.0.1' }])
      }
      if (path === '/api/scan/start') {
        return jsonResponse({ status: 'started' })
      }
      if (path === '/api/missing') {
        return new Response(JSON.stringify({ error: 'not found' }), { status: 404, headers: { 'Content-Type': 'application/json' } })
      }
      if (path === '/api/wrong-content') {
        return new Response('<html>oops</html>', { status: 200, headers: { 'Content-Type': 'text/html' } })
      }
      throw new Error(`unexpected fetch: ${path}`)
    }))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('GETs without csrf token', async () => {
    const result = await api.getDevices()
    expect(result).toEqual([{ mac: 'AA', ip: '10.0.0.1' }])
    expect(calls).toHaveLength(1)
    expect(calls[0].init?.method).toBe('GET')
    expect(calls[0].init?.headers).not.toHaveProperty('X-CSRF-Token')
  })

  it('POST fetches csrf-token then includes header', async () => {
    const result = await api.scanStart()
    expect(result).toEqual({ status: 'started' })
    expect(calls).toHaveLength(2)
    expect(calls[0].input).toBe('/api/csrf-token')
    expect(calls[1].input).toBe('/api/scan/start')
    const headers = calls[1].init?.headers as Record<string, string>
    expect(headers['X-CSRF-Token']).toBe('test-token')
    expect(headers['Content-Type']).toBe('application/json')
  })

  it('throws APIError with status + detail on non-ok responses', async () => {
    await expect(() =>
      // @ts-expect-error - exercising error path with arbitrary path
      (api as unknown as { _req: (m: string, p: string) => Promise<unknown> })._req,
    ).toBeDefined()

    // Use a real call that maps to /api/missing via a known endpoint signature.
    // We force it through by mocking deleteCredential against the missing path.
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const path = typeof input === 'string' ? input : input.toString()
      if (path === '/api/csrf-token') {
        return jsonResponse({ csrf_token: 'tok' })
      }
      return new Response(JSON.stringify({ error: 'not found' }), { status: 404, headers: { 'Content-Type': 'application/json' } })
    }))

    try {
      await api.deleteCredential('does-not-exist')
      expect.unreachable('should have thrown')
    } catch (err) {
      expect(err).toBeInstanceOf(APIError)
      const e = err as APIError
      expect(e.status).toBe(404)
      expect(e.message).toBe('not found')
      expect(e.method).toBe('DELETE')
    }
  })

  it('throws APIError when response is not JSON', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      return new Response('<html></html>', { status: 200, headers: { 'Content-Type': 'text/html' } })
    }))
    try {
      await api.getDevices()
      expect.unreachable('should have thrown')
    } catch (err) {
      expect(err).toBeInstanceOf(APIError)
      const e = err as APIError
      expect(e.message).toMatch(/expected JSON/i)
    }
  })
})
