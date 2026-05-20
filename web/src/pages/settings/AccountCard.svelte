<!--
  Operator account card for the Settings page. Changes the login username
  and/or password via /api/account/credentials. The endpoint verifies the
  current password and revokes all sessions on success, so the SPA bounces to
  the login screen afterward.
-->
<script lang="ts">
  import { APIError, api } from '../../lib/api';
  import { navigate } from '../../lib/stores';

  let username = '';
  let currentPassword = '';
  let newPassword = '';
  let confirm = '';
  let error = '';
  let busy = false;

  async function submit() {
    error = '';
    if (newPassword.length < 8) {
      error = 'New password must be at least 8 characters.';
      return;
    }
    if (newPassword !== confirm) {
      error = 'New passwords do not match.';
      return;
    }
    busy = true;
    try {
      await api.changeCredentials(currentPassword, username.trim(), newPassword);
      // Sessions were revoked server-side; return to the login screen.
      navigate('/login');
    } catch (err) {
      error = err instanceof APIError ? err.message : (err as Error).message;
    } finally {
      busy = false;
    }
  }
</script>

<div class="card bg-dark border-secondary h-100">
  <div class="card-body">
    <h2 class="h5">Operator Account</h2>
    <p class="text-secondary small mb-3">
      Change the login username or password. You'll be signed out and asked to log in again with the
      new credentials. Leave the username blank to keep it unchanged.
    </p>

    {#if error}
      <div class="alert alert-danger py-2 small">{error}</div>
    {/if}

    <div class="mb-3">
      <label class="form-label" for="account-username">New username (optional)</label>
      <input id="account-username" class="form-control" bind:value={username} placeholder="admin" />
    </div>
    <div class="mb-3">
      <label class="form-label" for="account-current">Current password</label>
      <input
        id="account-current"
        class="form-control"
        type="password"
        autocomplete="current-password"
        bind:value={currentPassword}
      />
    </div>
    <div class="mb-3">
      <label class="form-label" for="account-new">New password</label>
      <input
        id="account-new"
        class="form-control"
        type="password"
        autocomplete="new-password"
        bind:value={newPassword}
      />
    </div>
    <div class="mb-3">
      <label class="form-label" for="account-confirm">Confirm new password</label>
      <input
        id="account-confirm"
        class="form-control"
        type="password"
        autocomplete="new-password"
        bind:value={confirm}
        on:keydown={(e) => e.key === 'Enter' && submit()}
      />
    </div>
    <button class="btn btn-warning text-dark" on:click={submit} disabled={busy}>
      {busy ? 'Saving…' : 'Change credentials'}
    </button>
  </div>
</div>
