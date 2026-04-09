import type { AppSettings, DebugLogResponse, Device, FirmwareStatus, LogEntry, ScanStatus } from './types'

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  return res.json()
}

export const api = {
  login: (username: string, password: string) => req<{ ok: true }>('POST', '/login', { username, password }),
  getDevices: () => req<Device[]>('GET', '/api/devices'),
  refreshDevices: () => req<Device[]>('GET', '/api/devices/refresh'),
  refreshDevice: (target: string) => req<Device[]>('POST', '/api/devices/refresh-one', { target }),
  forgetDevice: (target: string) => req<{ ok: true }>('POST', '/api/devices/forget', { target }),
  bulk: (payload: unknown) => req<{ results: Array<{ mac: string; ip: string; status: string }> }>('POST', '/api/bulk', payload),
  scanStart: () => req<{ status: string }>('GET', '/api/scan/start'),
  scanStatus: () => req<ScanStatus>('GET', '/api/scan/status'),
  scanConfirm: (macs?: string[]) => req<{ ok: true; count: number }>('POST', '/api/scan/confirm', macs ? { macs } : {}),
  firmwareCheck: (stage: string) => req<{ status: string; total: number }>('GET', `/api/firmware/check?stage=${stage}`),
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
