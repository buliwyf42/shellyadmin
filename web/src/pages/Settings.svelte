<script lang="ts">
  import { onMount } from 'svelte';
  import { APIError, api } from '../lib/api';
  import type { AppSettings, BackupExport, Credential, ImportReport } from '../lib/types';
  import ErrorNotice from '../components/ErrorNotice.svelte';
  import AccountCard from './settings/AccountCard.svelte';
  import TOTPCard from './settings/TOTPCard.svelte';
  import TokensCard from './settings/TokensCard.svelte';

  let settings: AppSettings = {
    subnets: [],
    scan_timeout: 2,
    refresh_timeout: 5,
    scan_concurrency: 64,
    enable_mdns: false,
    advanced_mode_enabled: false,
    gen2_badge_class: 'bg-warning text-dark',
    gen3_badge_class: 'bg-success',
    gen4_badge_class: 'bg-info text-dark',
    gen_frozen_badge_class: 'bg-warning text-dark',
    firmware_install_timeout: 600,
    firmware_install_quiet_period: 150,
    firmware_install_poll_interval: 5,
    firmware_check_interval: 0,
    mcp_enabled: false,
    mcp_token: '',
    mcp_managed_by_env: false,
    compliance: {},
  };

  // Sane preset cadences for the scheduled firmware check; the backend
  // accepts any positive seconds so an operator can dial in something custom
  // via the JSON API. The dropdown stays small.
  const firmwareCheckCadences: { value: number; label: string }[] = [
    { value: 0, label: 'Off (manual only)' },
    { value: 60 * 60, label: 'Hourly' },
    { value: 6 * 60 * 60, label: 'Every 6 hours' },
    { value: 12 * 60 * 60, label: 'Every 12 hours' },
    { value: 24 * 60 * 60, label: 'Daily' },
    { value: 7 * 24 * 60 * 60, label: 'Weekly' },
  ];

  const badgeColorOptions: { value: string; label: string }[] = [
    { value: 'bg-success', label: 'Green (success)' },
    { value: 'bg-info text-dark', label: 'Cyan (info)' },
    { value: 'bg-primary', label: 'Blue (primary)' },
    { value: 'bg-warning text-dark', label: 'Amber (warning)' },
    { value: 'bg-danger', label: 'Red (danger)' },
    { value: 'bg-secondary', label: 'Grey (secondary)' },
    { value: 'bg-dark border border-light', label: 'Dark (outlined)' },
  ];
  let credentials: Credential[] = [];
  let error = '';
  let errorDetails = '';
  let status = '';
  let includeSecrets = false;
  let importText = '';
  let importReport: ImportReport | null = null;
  let pendingImport: BackupExport | null = null;

  function captureError(err: unknown) {
    if (err instanceof APIError) {
      error = err.message;
      errorDetails = `${err.method} ${err.path} -> ${err.status}\n${JSON.stringify(err.detail ?? {}, null, 2)}`;
      return;
    }
    error = (err as Error).message;
    errorDetails = String(err);
  }

  function setStatus(message: string) {
    status = message;
    setTimeout(() => {
      if (status === message) status = '';
    }, 2000);
  }

  function clearMessages() {
    error = '';
    errorDetails = '';
  }

  function downloadJSON(filename: string, payload: unknown) {
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = filename;
    link.click();
    URL.revokeObjectURL(url);
  }

  async function load() {
    clearMessages();
    try {
      settings = await api.getSettings();
      credentials = await api.listCredentials();
    } catch (err) {
      captureError(err);
    }
  }

  async function saveSettings() {
    clearMessages();
    try {
      await api.saveSettings(settings);
      // Re-load so the MCP running/stopped badge and the redacted token
      // placeholder reflect the new server state immediately. Other
      // form values round-trip unchanged.
      settings = await api.getSettings();
      mcpTokenVisible = false;
      setStatus('Settings saved');
    } catch (err) {
      captureError(err);
    }
  }

  /**
   * Generate a fresh 64-hex-char MCP token (32 bytes of CSPRNG-backed
   * randomness). Same length as `openssl rand -hex 32`. The new value
   * replaces the placeholder text in the input but is not persisted
   * until Save Settings.
   */
  function generateMCPToken() {
    const bytes = new Uint8Array(32);
    crypto.getRandomValues(bytes);
    const hex = Array.from(bytes)
      .map((b) => b.toString(16).padStart(2, '0'))
      .join('');
    settings = { ...settings, mcp_token: hex };
    mcpTokenVisible = true;
  }

  async function copyMCPToken() {
    if (!settings.mcp_token || settings.mcp_token === '<set>') return;
    try {
      await navigator.clipboard.writeText(settings.mcp_token);
      setStatus('Token copied');
    } catch (err) {
      captureError(err);
    }
  }

  function clearMCPToken() {
    settings = { ...settings, mcp_token: '' };
    mcpTokenVisible = false;
  }

  let mcpTokenVisible = false;

  async function exportBackup() {
    clearMessages();
    try {
      const confirm = includeSecrets ? 'export-plaintext-secrets' : '';
      const payload = await api.exportBackup(includeSecrets, confirm);
      const suffix = includeSecrets ? 'with-secrets' : 'redacted';
      downloadJSON(`shellyadmin-backup-${suffix}.json`, payload);
      setStatus(includeSecrets ? 'Exported with plaintext secrets' : 'Exported redacted backup');
    } catch (err) {
      captureError(err);
    }
  }

  async function dryRunImport() {
    clearMessages();
    importReport = null;
    pendingImport = null;
    try {
      const parsed = JSON.parse(importText) as BackupExport;
      const report = await api.importBackup(parsed, false);
      importReport = report;
      pendingImport = parsed;
      setStatus('Dry-run completed');
    } catch (err) {
      captureError(err);
    }
  }

  async function applyImport() {
    if (!pendingImport) {
      error = 'Run dry-run first';
      errorDetails = '';
      return;
    }
    clearMessages();
    try {
      importReport = await api.importBackup(pendingImport, true);
      await load();
      setStatus('Import applied');
    } catch (err) {
      captureError(err);
    }
  }

  onMount(() => void load());
</script>

<ErrorNotice summary={error} details={errorDetails} />
{#if status}
  <div class="alert alert-secondary">{status}</div>
{/if}

<div class="row g-3">
  <!-- Card 1: Discovery & Refresh -->
  <div class="col-lg-6">
    <div class="card bg-dark border-secondary h-100">
      <div class="card-body">
        <h2 class="h5">Discovery & Refresh</h2>
        <p class="text-secondary small mb-3">
          Where to look for devices and how aggressively. mDNS works best on hosts that can receive
          local multicast traffic; Docker setups may need host networking for reliable results.
        </p>
        <label class="form-label" for="settings-subnets">Subnets (one per line)</label>
        <textarea
          id="settings-subnets"
          class="form-control mb-3"
          rows="5"
          value={settings.subnets.join('\n')}
          on:input={(e) =>
            (settings.subnets = (e.currentTarget as HTMLTextAreaElement).value
              .split('\n')
              .map((v) => v.trim())
              .filter(Boolean))}></textarea>
        <div class="row g-3">
          <div class="col-md-4">
            <label class="form-label" for="settings-scan-timeout">Scan timeout (s)</label><input
              id="settings-scan-timeout"
              class="form-control"
              type="number"
              bind:value={settings.scan_timeout}
            />
          </div>
          <div class="col-md-4">
            <label class="form-label" for="settings-refresh-timeout">Refresh timeout (s)</label
            ><input
              id="settings-refresh-timeout"
              class="form-control"
              type="number"
              bind:value={settings.refresh_timeout}
            />
          </div>
          <div class="col-md-4">
            <label class="form-label" for="settings-scan-concurrency">Concurrency</label><input
              id="settings-scan-concurrency"
              class="form-control"
              type="number"
              bind:value={settings.scan_concurrency}
            />
          </div>
        </div>
        <label class="d-flex gap-2 align-items-center mt-3">
          <input type="checkbox" class="form-check-input" bind:checked={settings.enable_mdns} />
          <span>Enable mDNS-assisted discovery</span>
        </label>

        <button class="btn btn-warning text-dark mt-3" on:click={saveSettings}>Save Settings</button
        >
      </div>
    </div>
  </div>

  <!-- Card 2: Firmware -->
  <div class="col-lg-6">
    <div class="card bg-dark border-secondary h-100">
      <div class="card-body">
        <h2 class="h5">Firmware</h2>
        <p class="text-secondary small mb-3">
          The scheduled check skips a tick if a manual Check Firmware is already running. Install
          timeout is per-device, not job-total — a fleet of 50 devices still completes in minutes
          thanks to bounded parallel installs. Lower the install poll for snappier feedback on a
          small fleet; raise it to be gentler on slow devices.
        </p>
        <div class="row g-3">
          <div class="col-md-4">
            <label class="form-label" for="settings-firmware-check-interval">
              Scheduled check
            </label>
            <select
              id="settings-firmware-check-interval"
              class="form-select"
              bind:value={settings.firmware_check_interval}
              title="When non-zero, ShellyAdmin runs Check Firmware on every device at this cadence."
            >
              {#each firmwareCheckCadences as opt (opt.value)}
                <option value={opt.value}>{opt.label}</option>
              {/each}
            </select>
          </div>
          <div class="col-md-4">
            <label class="form-label" for="settings-firmware-install-timeout">
              Install timeout (s)
            </label>
            <input
              id="settings-firmware-install-timeout"
              class="form-control"
              type="number"
              min="60"
              step="30"
              bind:value={settings.firmware_install_timeout}
              title="Per-device timeout: how long the install_job waits for a device to reboot onto the new firmware before marking it 'unknown'. Default 600 (10 min). Raised automatically to at least the quiet period + 150s."
            />
          </div>
          <div class="col-md-4">
            <label class="form-label" for="settings-firmware-install-quiet-period">
              Install quiet period (s)
            </label>
            <input
              id="settings-firmware-install-quiet-period"
              class="form-control"
              type="number"
              min="0"
              max="600"
              step="10"
              bind:value={settings.firmware_install_quiet_period}
              title="How long the install_job leaves a device completely alone after triggering the update. Default 150. An in-flight OTA has very little heap to spare — answering RPCs during the download stalls it at 0%, so polling only starts once this has elapsed. Lower it only if you know the device downloads faster."
            />
          </div>
          <div class="col-md-4">
            <label class="form-label" for="settings-firmware-install-poll-interval">
              Install poll (s)
            </label>
            <input
              id="settings-firmware-install-poll-interval"
              class="form-control"
              type="number"
              min="1"
              max="60"
              step="1"
              bind:value={settings.firmware_install_poll_interval}
              title="How often the install_job re-queries a device's firmware version once the quiet period has elapsed. Default 5; bounded 1–60."
            />
          </div>
        </div>

        <button class="btn btn-warning text-dark mt-3" on:click={saveSettings}>Save Settings</button
        >
      </div>
    </div>
  </div>

  <!-- Card 3: MCP server (read-only API for LLM agents) -->
  <div class="col-lg-6">
    <div class="card bg-dark border-secondary h-100">
      <div class="card-body">
        <h2 class="h5 d-flex align-items-center gap-2">
          MCP Server
          <span class="badge bg-secondary">Read-only</span>
          {#if settings.mcp_running}
            <span class="badge bg-success" title="MCP listener is currently active">Running</span>
          {:else}
            <span class="badge bg-secondary" title="MCP listener is not running">Stopped</span>
          {/if}
        </h2>
        <p class="text-secondary small mb-3">
          Exposes a Model Context Protocol server on port 8081 (default) so LLM-driven agents can
          introspect the fleet — list devices, check firmware status, read compliance, inspect logs.
          State-changing operations are deliberately not exposed. <strong
            >Saves apply immediately</strong
          > — the listener starts, stops, or rotates its token without restarting the container.
        </p>

        {#if settings.mcp_managed_by_env}
          <div class="alert alert-secondary py-2 small mb-3">
            <strong>Managed by environment variable.</strong> The
            <code>SHELLYADMIN_MCP_TOKEN</code>
            env var is set on this container, so the fields below are ignored. Unset the env var (and
            restart) to switch to settings-driven configuration.
          </div>
        {/if}

        <label class="d-flex gap-2 align-items-center mb-3">
          <input
            type="checkbox"
            class="form-check-input"
            bind:checked={settings.mcp_enabled}
            disabled={settings.mcp_managed_by_env}
          />
          <span>Enable MCP server</span>
        </label>

        <label class="form-label" for="settings-mcp-token">Token</label>
        <div class="input-group mb-2">
          <input
            id="settings-mcp-token"
            class="form-control font-monospace"
            type={mcpTokenVisible ? 'text' : 'password'}
            placeholder="64-character hex recommended"
            autocomplete="off"
            spellcheck="false"
            bind:value={settings.mcp_token}
            disabled={settings.mcp_managed_by_env}
          />
          <button
            class="btn btn-outline-light"
            type="button"
            on:click={() => (mcpTokenVisible = !mcpTokenVisible)}
            disabled={settings.mcp_managed_by_env || !settings.mcp_token}
            title={mcpTokenVisible ? 'Hide token' : 'Show token'}
            aria-label={mcpTokenVisible ? 'Hide token' : 'Show token'}
          >
            {mcpTokenVisible ? 'Hide' : 'Show'}
          </button>
          <button
            class="btn btn-outline-light"
            type="button"
            on:click={generateMCPToken}
            disabled={settings.mcp_managed_by_env}
            title="Replace with a fresh 32-byte CSPRNG token (64 hex chars)"
          >
            Generate
          </button>
          <button
            class="btn btn-outline-light"
            type="button"
            on:click={copyMCPToken}
            disabled={settings.mcp_managed_by_env ||
              !settings.mcp_token ||
              settings.mcp_token === '<set>'}
            title="Copy plaintext token to clipboard"
          >
            Copy
          </button>
          <button
            class="btn btn-outline-danger"
            type="button"
            on:click={clearMCPToken}
            disabled={settings.mcp_managed_by_env || !settings.mcp_token}
            title="Clear stored token (will require a fresh one before re-enabling)"
          >
            Clear
          </button>
        </div>
        <p class="text-secondary small mb-3">
          {#if settings.mcp_token === '<set>'}
            A token is currently configured. Click <em>Generate</em> to rotate it (Save Settings
            applies the new token without restarting), or <em>Clear</em> to remove it.
          {:else if settings.mcp_token}
            Token is set in the form but not yet saved. Use <em>Copy</em> to put it on the clipboard for
            your MCP client config, then click Save Settings.
          {:else}
            No token configured. The MCP server will not start until a token is set and saved.
          {/if}
        </p>

        <button
          class="btn btn-warning text-dark"
          on:click={saveSettings}
          disabled={settings.mcp_managed_by_env}
        >
          Save Settings
        </button>
      </div>
    </div>
  </div>

  <!-- Card 3b: Operator Account (first-run setup, change login) -->
  <div class="col-lg-6">
    <AccountCard />
  </div>

  <!-- Card 4: Two-Factor Authentication (T1, v0.3.0) -->
  <div class="col-lg-6">
    <TOTPCard />
  </div>

  <!-- Card 5: Personal Access Tokens (T3, v0.3.0) -->
  <div class="col-lg-12">
    <TokensCard />
  </div>

  <!-- Card 6: Display -->
  <div class="col-lg-6">
    <div class="card bg-dark border-secondary h-100">
      <div class="card-body">
        <h2 class="h5">Display</h2>
        <p class="text-secondary small mb-3">
          UI preferences. <em>Advanced mode</em> reveals power-user surfaces such as the raw JSON template
          editor on the Provision page; off by default so the guided form is the only entry point.
        </p>
        <label class="d-flex gap-2 align-items-center mb-3">
          <input
            type="checkbox"
            class="form-check-input"
            bind:checked={settings.advanced_mode_enabled}
          />
          <span>Enable advanced mode</span>
        </label>

        <h3 class="h6 mt-3">Generation badge colors</h3>
        <p class="text-secondary small mb-2">
          Used on the Devices and Firmware pages. Live preview:
          <span class={`badge ${settings.gen2_badge_class || 'bg-warning text-dark'}`}>Gen 2.x</span
          >
          <span class={`badge ${settings.gen3_badge_class || 'bg-success'}`}>Gen 3.x</span>
          <span class={`badge ${settings.gen4_badge_class || 'bg-info text-dark'}`}>Gen 4.x</span>
          <span class={`badge ${settings.gen_frozen_badge_class || 'bg-warning text-dark'}`}
            >Gen 2.x (frozen)</span
          >
        </p>
        <div class="row g-3">
          <div class="col-md-3">
            <label class="form-label" for="settings-gen2-badge">Gen 2.x</label>
            <select
              id="settings-gen2-badge"
              class="form-select"
              bind:value={settings.gen2_badge_class}
            >
              {#each badgeColorOptions as opt (opt.value)}
                <option value={opt.value}>{opt.label}</option>
              {/each}
            </select>
          </div>
          <div class="col-md-3">
            <label class="form-label" for="settings-gen3-badge">Gen 3.x</label>
            <select
              id="settings-gen3-badge"
              class="form-select"
              bind:value={settings.gen3_badge_class}
            >
              {#each badgeColorOptions as opt (opt.value)}
                <option value={opt.value}>{opt.label}</option>
              {/each}
            </select>
          </div>
          <div class="col-md-3">
            <label class="form-label" for="settings-gen4-badge">Gen 4.x</label>
            <select
              id="settings-gen4-badge"
              class="form-select"
              bind:value={settings.gen4_badge_class}
            >
              {#each badgeColorOptions as opt (opt.value)}
                <option value={opt.value}>{opt.label}</option>
              {/each}
            </select>
          </div>
          <div class="col-md-3">
            <label class="form-label" for="settings-gen-frozen-badge">Feature-frozen</label>
            <select
              id="settings-gen-frozen-badge"
              class="form-select"
              bind:value={settings.gen_frozen_badge_class}
            >
              {#each badgeColorOptions as opt (opt.value)}
                <option value={opt.value}>{opt.label}</option>
              {/each}
            </select>
          </div>
        </div>

        <button class="btn btn-warning text-dark mt-3" on:click={saveSettings}>Save Settings</button
        >
      </div>
    </div>
  </div>
  <div class="col-lg-12">
    <div class="card bg-dark border-secondary">
      <div class="card-body">
        <h2 class="h5">Backup (Settings + Templates + Groups)</h2>
        <p class="text-secondary">Restore is a two-step flow: dry-run first, then apply.</p>
        <div class="d-flex gap-2 align-items-center flex-wrap mb-3">
          <label class="d-flex gap-2 align-items-center mb-0">
            <input type="checkbox" class="form-check-input" bind:checked={includeSecrets} />
            Include plaintext secrets (requires explicit confirmation)
          </label>
          <button class="btn btn-outline-light" on:click={exportBackup}>Export JSON</button>
        </div>

        <label class="form-label" for="backup-import-json">Import JSON</label>
        <textarea
          id="backup-import-json"
          class="form-control font-monospace mb-2"
          rows="10"
          bind:value={importText}
          placeholder="Paste exported backup JSON here"></textarea>
        <div class="d-flex gap-2 flex-wrap">
          <button
            class="btn btn-outline-light"
            on:click={dryRunImport}
            disabled={!importText.trim()}>Dry Run</button
          >
          <button class="btn btn-warning text-dark" on:click={applyImport} disabled={!pendingImport}
            >Apply Import</button
          >
        </div>

        {#if importReport}
          <div class="mt-3">
            <h3 class="h6">Import Report</h3>
            <pre class="mb-0">{JSON.stringify(importReport, null, 2)}</pre>
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>
