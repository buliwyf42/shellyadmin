<script lang="ts">
  import { api } from '../lib/api';
  import { navigate } from '../lib/stores';

  let username = 'admin';
  let password = '';
  let confirm = '';
  let error = '';
  let saving = false;

  async function submit() {
    error = '';
    if (password.length < 8) {
      error = 'Password must be at least 8 characters.';
      return;
    }
    if (password !== confirm) {
      error = 'Passwords do not match.';
      return;
    }
    saving = true;
    try {
      await api.setup(username.trim() || 'admin', password);
      navigate('/login');
    } catch (err) {
      error = (err as Error).message;
    } finally {
      saving = false;
    }
  }
</script>

<div class="min-vh-100 d-flex align-items-center justify-content-center bg-black">
  <div class="card bg-dark border-secondary shadow-lg login-card-width">
    <div class="card-body p-4">
      <h1 class="h3 mb-1">ShellyAdmin</h1>
      <p class="text-secondary">Create the operator account to finish setup.</p>
      {#if error}<div class="alert alert-danger py-2">{error}</div>{/if}

      <div class="mb-3">
        <label class="form-label" for="setup-username">Username</label>
        <input id="setup-username" class="form-control" bind:value={username} />
      </div>
      <div class="mb-3">
        <label class="form-label" for="setup-password">Password</label>
        <input id="setup-password" class="form-control" type="password" bind:value={password} />
      </div>
      <div class="mb-3">
        <label class="form-label" for="setup-confirm">Confirm password</label>
        <input
          id="setup-confirm"
          class="form-control"
          type="password"
          bind:value={confirm}
          on:keydown={(e) => e.key === 'Enter' && submit()}
        />
      </div>
      <button class="btn btn-warning text-dark w-100" disabled={saving} on:click={submit}>
        {saving ? 'Creating…' : 'Create account'}
      </button>
    </div>
  </div>
</div>
