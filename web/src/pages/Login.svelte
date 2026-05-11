<script lang="ts">
  import { APIError, api } from '../lib/api';
  import { navigate } from '../lib/stores';

  let username = 'admin';
  let password = '';
  let totpCode = '';
  let needsTOTP = false;
  let error = '';

  async function submit() {
    error = '';
    try {
      await api.login(username, password, needsTOTP ? totpCode.trim() : undefined);
      navigate('/');
    } catch (err) {
      if (err instanceof APIError && err.status === 401) {
        const code = (err.detail as { error?: string } | null)?.error ?? '';
        if (code === 'totp_required') {
          // Password gate cleared but the operator is enrolled in 2FA.
          // Reveal the second-factor field and let them re-submit; do
          // not surface a "wrong credentials" error in this branch
          // because the password WAS right.
          needsTOTP = true;
          error = '';
          // Defer focus to the next tick so the input is mounted.
          setTimeout(() => {
            document.getElementById('login-totp-code')?.focus();
          }, 0);
          return;
        }
        if (code === 'invalid_totp_code') {
          needsTOTP = true;
          totpCode = '';
          error = 'Invalid two-factor code';
          return;
        }
      }
      error = (err as Error).message;
    }
  }

  function resetTOTP() {
    needsTOTP = false;
    totpCode = '';
    error = '';
  }
</script>

<div class="min-vh-100 d-flex align-items-center justify-content-center bg-black">
  <div class="card bg-dark border-secondary shadow-lg login-card-width">
    <div class="card-body p-4">
      <h1 class="h3 mb-3">ShellyAdmin</h1>
      <p class="text-secondary">Fleet operations for your Shelly network.</p>
      {#if error}<div class="alert alert-danger py-2">{error}</div>{/if}

      {#if !needsTOTP}
        <div class="mb-3">
          <label class="form-label" for="login-username">Username</label>
          <input id="login-username" class="form-control" bind:value={username} />
        </div>
        <div class="mb-3">
          <label class="form-label" for="login-password">Password</label>
          <input
            id="login-password"
            class="form-control"
            type="password"
            bind:value={password}
            on:keydown={(e) => e.key === 'Enter' && submit()}
          />
        </div>
        <button class="btn btn-warning text-dark w-100" on:click={submit}>Sign In</button>
      {:else}
        <p class="text-secondary small mb-2">
          Two-factor authentication is enabled. Enter the 6-digit code from your authenticator app,
          or one of your backup codes.
        </p>
        <div class="mb-3">
          <label class="form-label" for="login-totp-code">Code</label>
          <input
            id="login-totp-code"
            class="form-control font-monospace"
            inputmode="text"
            autocomplete="one-time-code"
            spellcheck="false"
            bind:value={totpCode}
            on:keydown={(e) => e.key === 'Enter' && submit()}
          />
        </div>
        <button class="btn btn-warning text-dark w-100 mb-2" on:click={submit}>Verify</button>
        <button class="btn btn-link w-100" on:click={resetTOTP}>← Use a different account</button>
      {/if}
    </div>
  </div>
</div>
