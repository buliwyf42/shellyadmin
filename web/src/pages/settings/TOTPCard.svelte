<!--
  TOTP 2FA management card for the Settings page. Drives the per-
  operator enrollment / disable flow against /api/totp/*. Three view
  states:

    1. notEnrolled — shows the "Enable 2FA" button.
    2. enrolling  — surfaces the freshly-minted secret + otpauth URI +
                    10 single-use backup codes, with a code input that
                    commits the enrollment via /api/totp/verify-enroll.
    3. enrolled   — shows enrollment metadata + a disable form that
                    requires a fresh TOTP / backup code (so a stolen
                    cookie cannot quietly turn 2FA off).

  T1 in v0.3.0 (docs/plans/phase-4c-auth-strategics.md, Block 4c.1).
  QR-code rendering landed post-v0.3.1 — operators can now scan the
  enrolment with a phone-based authenticator instead of typing the
  base32 secret by hand. Manual entry stays as a fallback for
  desktop-managed password vaults (Bitwarden, 1Password) that don't
  scan.
-->
<script lang="ts">
  import { onMount, tick } from 'svelte';
  import QRCode from 'qrcode';
  import { APIError, api } from '../../lib/api';
  import type { TOTPEnrollResponse, TOTPStatus } from '../../lib/types';

  let status: TOTPStatus | null = null;
  let pending: TOTPEnrollResponse | null = null;
  let confirmCode = '';
  let disableCode = '';
  let error = '';
  let statusMessage = '';
  let busy = false;

  function flashStatus(msg: string) {
    statusMessage = msg;
    setTimeout(() => {
      if (statusMessage === msg) statusMessage = '';
    }, 2500);
  }

  function captureError(err: unknown) {
    if (err instanceof APIError) {
      error = err.message;
      return;
    }
    error = (err as Error).message;
  }

  async function loadStatus() {
    error = '';
    try {
      status = await api.totp.status();
    } catch (err) {
      captureError(err);
    }
  }

  async function startEnrollment() {
    error = '';
    busy = true;
    try {
      pending = await api.totp.enroll();
      confirmCode = '';
      // Wait for the canvas to mount, then render the QR from the
      // freshly-minted otpauth URI. errorCorrectionLevel='M' is a
      // safe default (15% redundancy — recoverable on a phone camera
      // even with mild glare on the screen).
      await tick();
      await renderQR(pending.otpauth_uri);
    } catch (err) {
      captureError(err);
    } finally {
      busy = false;
    }
  }

  // qrCanvas is bound by the <canvas> element below; renderQR draws
  // the otpauth URI onto it via the qrcode library. Width chosen so
  // the QR scans cleanly on a phone camera held ~20cm from a
  // standard-DPI monitor.
  let qrCanvas: HTMLCanvasElement | null = null;

  async function renderQR(uri: string) {
    if (!qrCanvas) return;
    try {
      await QRCode.toCanvas(qrCanvas, uri, {
        width: 220,
        margin: 2,
        errorCorrectionLevel: 'M',
        color: { dark: '#000000', light: '#ffffff' },
      });
    } catch (err) {
      // QR render failure is non-fatal — the operator can still type
      // the secret manually. Surface a status message but don't break
      // the enrollment flow.
      captureError(err);
    }
  }

  async function confirmEnrollment() {
    error = '';
    if (!pending) return;
    const code = confirmCode.trim();
    if (!code) {
      error = 'Enter the 6-digit code shown in your authenticator app';
      return;
    }
    busy = true;
    try {
      await api.totp.verifyEnroll(code);
      pending = null;
      confirmCode = '';
      flashStatus('Two-factor authentication enabled');
      await loadStatus();
    } catch (err) {
      captureError(err);
    } finally {
      busy = false;
    }
  }

  function cancelEnrollment() {
    // The pending material lives in the session cookie until it's
    // overwritten by the next Enroll OR cleared by Logout. Dropping
    // the form-local copy is fine; a stale session entry is harmless.
    pending = null;
    confirmCode = '';
    error = '';
  }

  async function disableTOTP() {
    error = '';
    const code = disableCode.trim();
    if (!code) {
      error = 'Enter a TOTP or backup code to confirm';
      return;
    }
    busy = true;
    try {
      await api.totp.disable(code);
      disableCode = '';
      flashStatus('Two-factor authentication disabled');
      await loadStatus();
    } catch (err) {
      captureError(err);
    } finally {
      busy = false;
    }
  }

  async function copyToClipboard(text: string, label: string) {
    try {
      await navigator.clipboard.writeText(text);
      flashStatus(`${label} copied`);
    } catch (err) {
      captureError(err);
    }
  }

  function downloadBackupCodes() {
    if (!pending) return;
    const body = [
      'ShellyAdmin two-factor backup codes',
      '',
      'Keep these somewhere safe. Each code works exactly once.',
      'Use a backup code in the login form when your authenticator',
      'device is unavailable.',
      '',
      ...pending.backup_codes,
      '',
    ].join('\n');
    const blob = new Blob([body], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'shellyadmin-2fa-backup-codes.txt';
    link.click();
    URL.revokeObjectURL(url);
  }

  onMount(() => void loadStatus());
</script>

<div class="card bg-dark border-secondary h-100">
  <div class="card-body">
    <h2 class="h5 d-flex align-items-center gap-2">
      Two-Factor Authentication
      {#if status?.enrolled}
        <span class="badge bg-success" title="2FA is active on this account">Enabled</span>
      {:else if pending}
        <span class="badge bg-warning text-dark" title="Pending verification">Enrolling</span>
      {:else}
        <span class="badge bg-secondary" title="2FA is not configured">Disabled</span>
      {/if}
    </h2>
    <p class="text-secondary small mb-3">
      TOTP-based second factor compatible with Google Authenticator, 1Password, Authy, and
      Bitwarden. Once enrolled, the login flow requires a 6-digit code in addition to your password.
      Ten single-use backup codes are issued at enrollment for use when your authenticator device is
      unavailable.
    </p>

    {#if error}
      <div class="alert alert-danger py-2 small">{error}</div>
    {/if}
    {#if statusMessage}
      <div class="alert alert-secondary py-2 small">{statusMessage}</div>
    {/if}

    {#if pending}
      <!-- Mid-enrollment: the secret + otpauth URI + backup codes are
           on the wire exactly once. The operator scans / types them
           into their authenticator and verifies a fresh code. -->
      <h3 class="h6">Step 1 — Add to your authenticator</h3>
      <p class="text-secondary small mb-2">
        Scan the QR code with your authenticator app, or type the secret manually if you're using a
        desktop password manager that doesn't scan.
      </p>
      <div class="d-flex justify-content-center mb-3">
        <canvas
          bind:this={qrCanvas}
          width="220"
          height="220"
          aria-label="TOTP enrollment QR code"
          class="bg-white rounded p-2"
        ></canvas>
      </div>
      <details class="mb-3">
        <summary class="text-secondary small">Manual entry / otpauth link</summary>
        <label class="form-label small mt-2" for="totp-manual-secret">Manual entry secret</label>
        <div class="input-group input-group-sm mb-2">
          <input
            id="totp-manual-secret"
            class="form-control font-monospace"
            readonly
            value={pending.secret}
          />
          <button
            class="btn btn-outline-light"
            type="button"
            on:click={() => copyToClipboard(pending!.secret, 'Secret')}
          >
            Copy
          </button>
        </div>
        <label class="form-label small" for="totp-otpauth-uri">otpauth:// URI</label>
        <div class="input-group input-group-sm">
          <input
            id="totp-otpauth-uri"
            class="form-control font-monospace"
            readonly
            value={pending.otpauth_uri}
          />
          <button
            class="btn btn-outline-light"
            type="button"
            on:click={() => copyToClipboard(pending!.otpauth_uri, 'otpauth URI')}
          >
            Copy
          </button>
        </div>
      </details>

      <h3 class="h6">Step 2 — Save your backup codes</h3>
      <p class="text-secondary small mb-2">
        These won't be shown again. Each code works exactly once. Store them somewhere safe (a
        password manager works).
      </p>
      <ul
        class="list-unstyled font-monospace small bg-black border border-secondary rounded p-2 mb-2"
      >
        {#each pending.backup_codes as code (code)}
          <li>{code}</li>
        {/each}
      </ul>
      <button
        class="btn btn-outline-light btn-sm mb-3"
        type="button"
        on:click={downloadBackupCodes}
      >
        Download as text
      </button>

      <h3 class="h6">Step 3 — Verify</h3>
      <p class="text-secondary small mb-2">
        Enter the 6-digit code your authenticator is showing right now to commit the enrollment.
      </p>
      <div class="mb-3">
        <label class="form-label" for="totp-enroll-code">Code</label>
        <input
          id="totp-enroll-code"
          class="form-control font-monospace"
          inputmode="numeric"
          autocomplete="one-time-code"
          spellcheck="false"
          bind:value={confirmCode}
          on:keydown={(e) => e.key === 'Enter' && confirmEnrollment()}
        />
      </div>
      <div class="d-flex gap-2">
        <button class="btn btn-warning text-dark" on:click={confirmEnrollment} disabled={busy}>
          Verify & Enable
        </button>
        <button class="btn btn-outline-light" on:click={cancelEnrollment} disabled={busy}>
          Cancel
        </button>
      </div>
    {:else if status?.enrolled}
      <!-- Enrolled: show metadata + the disable form. -->
      <dl class="row mb-3 small">
        <dt class="col-sm-5 text-secondary">Enrolled</dt>
        <dd class="col-sm-7 font-monospace">{status.enrolled_at || '—'}</dd>
        <dt class="col-sm-5 text-secondary">Last verified</dt>
        <dd class="col-sm-7 font-monospace">{status.last_verified_at || '—'}</dd>
        <dt class="col-sm-5 text-secondary">Backup codes remaining</dt>
        <dd class="col-sm-7">{status.backup_codes_left ?? 0} of 10</dd>
      </dl>
      <p class="text-secondary small mb-2">
        Disabling 2FA requires a fresh TOTP code or one of your unused backup codes. A stolen
        session cookie alone cannot turn 2FA off.
      </p>
      <div class="mb-3">
        <label class="form-label" for="totp-disable-code">Confirmation code</label>
        <input
          id="totp-disable-code"
          class="form-control font-monospace"
          inputmode="text"
          autocomplete="one-time-code"
          spellcheck="false"
          bind:value={disableCode}
          on:keydown={(e) => e.key === 'Enter' && disableTOTP()}
        />
      </div>
      <button class="btn btn-outline-danger" on:click={disableTOTP} disabled={busy}>
        Disable Two-Factor
      </button>
    {:else if status}
      <!-- Not enrolled, no pending material. Offer the enrollment button. -->
      <button class="btn btn-warning text-dark" on:click={startEnrollment} disabled={busy}>
        Enable Two-Factor
      </button>
    {/if}
  </div>
</div>
