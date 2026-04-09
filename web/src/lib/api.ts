import type { AppSettings, BackupExport, Credential, CredentialGroup, Device, FirmwareStatus, ImportReport, LogEntry, ScanStatus, TemplateRecord, VersionInfo } from './types'

const SAFE_METHODS = new Set(['GET', 'HEAD', 'OPTIONS'])
let csrfToken = ''
let csrfFetchInFlight: Promise<void> | null = null

export class APIError extends Error {
  method: string
  path: string
  status: number
  detail?: unknown

  constructor(method: string, path: string, status: number, message: string, detail?: unknown) {
    super(message)
    this.method = method
    this.path = path
    this.status = status
    this.detail = detail
  }
}

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
  const requiresCSRF = !SAFE_METHODS.has(method) && path !== '/login' && path !== '/api/login'
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
    const message = (err as { error?: string }).error || res.statusText
    throw new APIError(method, path, res.status, message, err)
  }
  return res.json()
}

export const api = {
  login: (username: string, password: string) => req<{ ok: true; csrf_token?: string }>('POST', '/api/login', { username, password }),
  logout: () => req<{ ok: true }>('POST', '/api/logout', {}),
  getDevices: () => req<Device[]>('GET', '/api/devices'),
  refreshDevices: () => req<Device[]>('POST', '/api/devices/refresh', {}),
  refreshDevice: (target: string) => req<Device[]>('POST', '/api/devices/refresh-one', { target }),
  forgetDevice: (target: string) => req<{ ok: true }>('POST', '/api/devices/forget', { target }),
  bulk: (payload: unknown) => req<{ results: Array<{ mac: string; ip: string; status: string }> }>('POST', '/api/bulk', payload),
  scanStart: () => req<{ status: string }>('POST', '/api/scan/start', {}),
  scanStatus: () => req<ScanStatus>('GET', '/api/scan/status'),
  getVersion: () => req<VersionInfo>('GET', '/api/version'),
  scanConfirm: (macs?: string[]) => req<{ ok: true; count: number }>('POST', '/api/scan/confirm', macs ? { macs } : {}),
  firmwareCheck: (stage: string) => req<{ status: string; total: number }>('POST', '/api/firmware/check', { stage }),
  firmwareStatus: () => req<FirmwareStatus>('GET', '/api/firmware/status'),
  firmwareUpdate: (macs: string[], stage: string) => req<Array<{ ip: string; status: string; detail: string }>>('POST', '/api/firmware/update', { macs, stage }),
  provision: (ips: string[], template: object, credentialRef = '') => req<Array<{ info: unknown; results: unknown[] }>>('POST', '/api/provision', { ips, template, credential_ref: credentialRef }),
  getSettings: () => req<AppSettings>('GET', '/api/settings'),
  saveSettings: (settings: AppSettings) => req<{ ok: true }>('POST', '/api/settings', settings),
  listTemplates: () => req<string[]>('GET', '/api/templates'),
  getTemplate: (name: string) => req<TemplateRecord>('GET', `/api/templates/${encodeURIComponent(name)}`),
  saveTemplate: (name: string, content: string, credentialRef = '') => req<{ ok: true }>('POST', `/api/templates/${encodeURIComponent(name)}`, { content, credential_ref: credentialRef }),
  deleteTemplate: (name: string) => req<{ ok: true }>('DELETE', `/api/templates/${encodeURIComponent(name)}`),
  listCredentials: () => req<Credential[]>('GET', '/api/credentials'),
  saveCredential: (credential: Credential) => req<{ ok: true }>('POST', '/api/credentials', credential),
  deleteCredential: (name: string) => req<{ ok: true }>('DELETE', `/api/credentials/${encodeURIComponent(name)}`),
  listCredentialGroups: () => req<CredentialGroup[]>('GET', '/api/credential-groups'),
  saveCredentialGroup: (group: CredentialGroup) => req<{ ok: true }>('POST', '/api/credential-groups', group),
  deleteCredentialGroup: (name: string) => req<{ ok: true }>('DELETE', `/api/credential-groups/${encodeURIComponent(name)}`),
  getCredentialGroupAssignments: () => req<{ assignments: Record<string, string> }>('GET', '/api/credential-groups/assignments'),
  saveCredentialGroupAssignments: (macs: string[], groupName: string) => req<{ ok: true }>('POST', '/api/credential-groups/assignments', { macs, group_name: groupName }),
  getLogs: (level = '', search = '') => req<LogEntry[]>('GET', `/api/logs?level=${encodeURIComponent(level)}&search=${encodeURIComponent(search)}`),
  exportBackup: (includeSecrets = false, confirm = '') => req<BackupExport>('GET', `/api/backup/export?include_secrets=${includeSecrets ? 'true' : 'false'}&confirm=${encodeURIComponent(confirm)}`),
  importBackup: (data: BackupExport, apply: boolean) => req<ImportReport>('POST', '/api/backup/import', { data, apply }),
}
