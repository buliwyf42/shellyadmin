import type {
  AppSettings,
  BackupExport,
  BulkActionPreview,
  BulkActionRequest,
  BulkActionResult,
  Credential,
  CredentialGroup,
  Device,
  DeviceAction,
  DeviceActionResult,
  DeviceDetail,
  DeviceExport,
  FirmwareStatus,
  FirmwareUpdateResult,
  ImportReport,
  LogEntry,
  ProvisionResult,
  ScanStatus,
  TemplateRecord,
  UploadUserCAResult,
  VersionInfo,
} from './types';

const SAFE_METHODS = new Set(['GET', 'HEAD', 'OPTIONS']);

// Network-retry policy: transient TypeError from fetch() (DNS blip, dropped
// socket, offline->online transition) is retried on idempotent methods only.
// HTTP status errors (4xx/5xx) are NEVER retried — the server already answered.
const NETWORK_RETRY_COUNT = 2;
const NETWORK_RETRY_BACKOFF_MS = [200, 400] as const;

let csrfToken = '';
let csrfFetchInFlight: Promise<void> | null = null;

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function isTransientNetworkError(err: unknown): boolean {
  // fetch() throws TypeError on network failure; APIError (from our own code)
  // is a semantic response-level error and must not be retried.
  return err instanceof TypeError;
}

export class APIError extends Error {
  method: string;
  path: string;
  status: number;
  detail?: unknown;

  constructor(method: string, path: string, status: number, message: string, detail?: unknown) {
    super(message);
    this.method = method;
    this.path = path;
    this.status = status;
    this.detail = detail;
  }
}

function updateCSRFToken(res: Response) {
  const token = res.headers.get('X-CSRF-Token');
  if (token) {
    csrfToken = token;
  }
}

async function ensureCSRFToken(): Promise<void> {
  if (csrfToken || csrfFetchInFlight) {
    return csrfFetchInFlight ?? Promise.resolve();
  }
  csrfFetchInFlight = (async () => {
    const res = await fetch('/api/csrf-token', {
      method: 'GET',
      credentials: 'same-origin',
    });
    updateCSRFToken(res);
    if (res.ok) {
      const payload = (await res.json().catch(() => null)) as { csrf_token?: string } | null;
      if (payload?.csrf_token) {
        csrfToken = payload.csrf_token;
      }
    }
  })().finally(() => {
    csrfFetchInFlight = null;
  });
  return csrfFetchInFlight;
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {};
  if (body) {
    headers['Content-Type'] = 'application/json';
  }
  const requiresCSRF = !SAFE_METHODS.has(method) && path !== '/login' && path !== '/api/login';
  if (requiresCSRF) {
    await ensureCSRFToken();
    if (csrfToken) {
      headers['X-CSRF-Token'] = csrfToken;
    }
  }

  const canRetry = SAFE_METHODS.has(method);
  let res: Response;
  let attempt = 0;
  while (true) {
    try {
      res = await fetch(path, {
        method,
        headers,
        body: body ? JSON.stringify(body) : undefined,
        credentials: 'same-origin',
      });
      break;
    } catch (err) {
      if (canRetry && attempt < NETWORK_RETRY_COUNT && isTransientNetworkError(err)) {
        await sleep(NETWORK_RETRY_BACKOFF_MS[attempt]);
        attempt++;
        continue;
      }
      throw err;
    }
  }
  updateCSRFToken(res);
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    const message = (err as { error?: string }).error || res.statusText;
    if (res.status === 401 && path !== '/api/login' && path !== '/api/csrf-token') {
      csrfToken = '';
      if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
        window.location.assign('/login');
      }
    }
    throw new APIError(method, path, res.status, message, err);
  }
  const contentType = res.headers.get('Content-Type') || '';
  if (!contentType.toLowerCase().includes('application/json')) {
    const bodyText = await res.text().catch(() => '');
    const snippet = bodyText.trim().slice(0, 160) || 'empty response body';
    throw new APIError(
      method,
      path,
      res.status,
      'expected JSON response but received non-JSON content',
      { content_type: contentType || 'unknown', body_snippet: snippet },
    );
  }
  return res.json();
}

async function fetchBlob(path: string): Promise<Blob> {
  const res = await fetch(path, { method: 'GET', credentials: 'same-origin' });
  updateCSRFToken(res);
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    const message = (err as { error?: string }).error || res.statusText;
    if (res.status === 401 && typeof window !== 'undefined' && window.location.pathname !== '/login') {
      csrfToken = '';
      window.location.assign('/login');
    }
    throw new APIError('GET', path, res.status, message, err);
  }
  return res.blob();
}

export function triggerDownload(filename: string, blob: Blob): void {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}

export const api = {
  login: (username: string, password: string) =>
    req<{ ok: true; csrf_token?: string }>('POST', '/api/login', { username, password }),
  logout: () => req<{ ok: true }>('POST', '/api/logout', {}),
  getDevices: () => req<Device[]>('GET', '/api/devices'),
  getDeviceDetail: (target: string) =>
    req<DeviceDetail>('GET', `/api/devices/${encodeURIComponent(target)}`),
  listDeviceActions: (target: string) =>
    req<{ actions: DeviceAction[] }>('GET', `/api/devices/${encodeURIComponent(target)}/actions`),
  runDeviceAction: (target: string, action: string, payload: { stage?: string } = {}) =>
    req<DeviceActionResult>(
      'POST',
      `/api/devices/${encodeURIComponent(target)}/actions/${encodeURIComponent(action)}`,
      payload,
    ),
  refreshDevices: () => req<Device[]>('POST', '/api/devices/refresh', {}),
  refreshDevice: (target: string) => req<Device[]>('POST', '/api/devices/refresh-one', { target }),
  forgetDevice: (target: string) => req<{ ok: true }>('POST', '/api/devices/forget', { target }),
  previewBulk: (payload: BulkActionRequest) =>
    req<{ dry_run: true; preview: BulkActionPreview }>('POST', '/api/bulk', {
      ...payload,
      dry_run: true,
    }),
  bulk: (payload: BulkActionRequest) =>
    req<{ dry_run: false; results: BulkActionResult[] }>('POST', '/api/bulk', {
      ...payload,
      dry_run: false,
    }),
  scanStart: () => req<{ status: string }>('POST', '/api/scan/start', {}),
  scanStatus: () => req<ScanStatus>('GET', '/api/scan/status'),
  getVersion: () => req<VersionInfo>('GET', '/api/version'),
  scanConfirm: (macs?: string[]) =>
    req<{ ok: true; count: number }>('POST', '/api/scan/confirm', macs ? { macs } : {}),
  firmwareCheck: (stage: string) =>
    req<{ status: string; total: number }>('POST', '/api/firmware/check', { stage }),
  firmwareStatus: () => req<FirmwareStatus>('GET', '/api/firmware/status'),
  firmwareUpdate: (macs: string[], stage: string) =>
    req<FirmwareUpdateResult[]>('POST', '/api/firmware/update', { macs, stage }),
  provision: (ips: string[], template: Record<string, unknown>, credentialRef = '') =>
    req<ProvisionResult[]>('POST', '/api/provision', {
      ips,
      template,
      credential_ref: credentialRef,
    }),
  uploadUserCA: (
    ips: string[],
    pem: string,
    kind: 'user_ca' | 'tls_client_cert' | 'tls_client_key' = 'user_ca',
  ) => req<UploadUserCAResult[]>('POST', '/api/provision/user-ca', { ips, pem, kind }),
  getSettings: () => req<AppSettings>('GET', '/api/settings'),
  saveSettings: (settings: AppSettings) => req<{ ok: true }>('POST', '/api/settings', settings),
  listTemplates: () => req<string[]>('GET', '/api/templates'),
  getTemplate: (name: string) =>
    req<TemplateRecord>('GET', `/api/templates/${encodeURIComponent(name)}`),
  saveTemplate: (name: string, content: string, credentialRef = '') =>
    req<{ ok: true }>('POST', `/api/templates/${encodeURIComponent(name)}`, {
      content,
      credential_ref: credentialRef,
    }),
  deleteTemplate: (name: string) =>
    req<{ ok: true }>('DELETE', `/api/templates/${encodeURIComponent(name)}`),
  listCredentials: () => req<Credential[]>('GET', '/api/credentials'),
  saveCredential: (credential: Credential) =>
    req<{ ok: true }>('POST', '/api/credentials', credential),
  deleteCredential: (name: string) =>
    req<{ ok: true }>('DELETE', `/api/credentials/${encodeURIComponent(name)}`),
  listCredentialGroups: () => req<CredentialGroup[]>('GET', '/api/credential-groups'),
  saveCredentialGroup: (group: CredentialGroup) =>
    req<{ ok: true }>('POST', '/api/credential-groups', group),
  deleteCredentialGroup: (name: string) =>
    req<{ ok: true }>('DELETE', `/api/credential-groups/${encodeURIComponent(name)}`),
  getCredentialGroupAssignments: () =>
    req<{ assignments: Record<string, string> }>('GET', '/api/credential-groups/assignments'),
  saveCredentialGroupAssignments: (macs: string[], groupName: string) =>
    req<{ ok: true }>('POST', '/api/credential-groups/assignments', {
      macs,
      group_name: groupName,
    }),
  getLogs: (level = '', search = '') =>
    req<LogEntry[]>(
      'GET',
      `/api/logs?level=${encodeURIComponent(level)}&search=${encodeURIComponent(search)}`,
    ),
  exportDevice: (target: string) =>
    req<DeviceExport>('GET', `/api/devices/${encodeURIComponent(target)}/export`),
  exportLogs: (level = '', search = '', format: 'csv' | 'ndjson' = 'csv') =>
    fetchBlob(
      `/api/logs/export?level=${encodeURIComponent(level)}&search=${encodeURIComponent(search)}&format=${encodeURIComponent(format)}`,
    ),
  clearLogs: () => req<{ ok: true; deleted: number }>('DELETE', '/api/logs'),
  exportBackup: (includeSecrets = false, confirm = '') =>
    req<BackupExport>(
      'GET',
      `/api/backup/export?include_secrets=${includeSecrets ? 'true' : 'false'}&confirm=${encodeURIComponent(confirm)}`,
    ),
  importBackup: (data: BackupExport, apply: boolean) =>
    req<ImportReport>('POST', '/api/backup/import', { data, apply }),
  getOpenAPIV1: () => req<Record<string, unknown>>('GET', '/api/openapi/v1.json'),
};
