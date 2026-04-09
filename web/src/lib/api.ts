import type { AppSettings, DebugLogResponse, Device, FirmwareStatus, LogEntry, ScanStatus } from './types'

const SAFE_METHODS = new Set(['GET', 'HEAD', 'OPTIONS'])
let csrfToken = ''
let csrfFetchInFlight: Promise<void> | null = null

function updateCSRFToken(res: Response) {
  const token = res.headers.get('X-CSRF-Token')
  if (token) {
    csrfToken = token
  }
}

async function ensureCSRFToken(): Promise<void> {
  if (csrfToken || csrfFetchInFlight) {
    return csrfFetchInFlight ?? Promise.resolve()
  }
  csrfFetchInFlight = (async () => {
    const res = await fetch('/api/csrf-token', {
      method: 'GET',
      credentials: 'same-origin',
    })
    updateCSRFToken(res)
    if (res.ok) {
      const payload = await res.json().catch(() => null) as { csrf_token?: string } | null
      if (payload?.csrf_token) {
        csrfToken = payload.csrf_token
      }
    }
  })().finally(() => {
    csrfFetchInFlight = null
  })
  return csrfFetchInFlight
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  if (body) {
    headers['Content-Type'] = 'application/json'
  }
  const requiresCSRF = !SAFE_METHODS.has(method) && path !== '/login'
  if (requiresCSRF) {
    await ensureCSRFToken()
    if (csrfToken) {
      headers['X-CSRF-Token'] = csrfToken
    }
  }

  const res = await fetch(path, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  })
  updateCSRFToken(res)
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  return res.json()
}

export const api = {
  login: (username: string, password: string) => req<{ ok: true; csrf_token?: string }>('POST', '/login', { username, password }),
  logout: () => req<{ ok: true }>('POST', '/logout', {}),
  getDevices: () => req<Device[]>('GET', '/api/devices'),
  refreshDevices: () => req<Device[]>('POST', '/api/devices/refresh', {}),
  refreshDevice: (target: string) => req<Device[]>('POST', '/api/devices/refresh-one', { target }),
  forgetDevice: (target: string) => req<{ ok: true }>('POST', '/api/devices/forget', { target }),
  bulk: (payload: unknown) => req<{ results: Array<{ mac: string; ip: string; status: string }> }>('POST', '/api/bulk', payload),
  scanStart: () => req<{ status: string }>('POST', '/api/scan/start', {}),
  scanStatus: () => req<ScanStatus>('GET', '/api/scan/status'),
  scanConfirm: (macs?: string[]) => req<{ ok: true; count: number }>('POST', '/api/scan/confirm', macs ? { macs } : {}),
  firmwareCheck: (stage: string) => req<{ status: string; total: number }>('POST', '/api/firmware/check', { stage }),
  firmwareStatus: () => req<FirmwareStatus>('GET', '/api/firmware/status'),
  firmwareUpdate: (macs: string[], stage: string) => req<Array<{ ip: string; status: string; detail: string }>>('POST', '/api/firmware/update', { macs, stage }),
  provision: (ips: string[], template: object) => req<Array<{ info: unknown; results: unknown[] }>>('POST', '/api/provision', { ips, template }),
  getSettings: () => req<AppSettings>('GET', '/api/settings'),
  saveSettings: (settings: AppSettings) => req<{ ok: true }>('POST', '/api/settings', settings),
  listTemplates: () => req<string[]>('GET', '/api/templates'),
  getTemplate: (name: string) => req<{ name: string; content: string }>('GET', `/api/templates/${encodeURIComponent(name)}`),
  saveTemplate: (name: string, content: string) => req<{ ok: true }>('POST', `/api/templates/${encodeURIComponent(name)}`, { content }),
  deleteTemplate: (name: string) => req<{ ok: true }>('DELETE', `/api/templates/${encodeURIComponent(name)}`),
  getLogs: (level = '', search = '') => req<LogEntry[]>('GET', `/api/logs?level=${encodeURIComponent(level)}&search=${encodeURIComponent(search)}`),
  getDebugLogs: (search = '', tail = 200) => req<DebugLogResponse>('GET', `/api/debug-logs?search=${encodeURIComponent(search)}&tail=${tail}`),
}
