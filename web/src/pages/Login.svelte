<script lang="ts">
  import { api } from '../lib/api';
  import { navigate } from '../lib/stores';

  let username = 'admin';
  let password = '';
  let error = '';

  async function submit() {
    error = '';
    try {
      await api.login(username, password);
      navigate('/');
    } catch (err) {
      error = (err as Error).message;
    }
  }
</script>

<div class="min-vh-100 d-flex align-items-center justify-content-center bg-black">
  <div class="card bg-dark border-secondary shadow-lg" style="width: min(28rem, 95vw)">
    <div class="card-body p-4">
      <h1 class="h3 mb-3">ShellyAdmin</h1>
      <p class="text-secondary">Fleet operations for your Shelly network.</p>
      {#if error}<div class="alert alert-danger py-2">{error}</div>{/if}
      <div class="mb-3">
        <label class="form-label" for="login-username">Username</label>
        <input id="login-username" class="form-control" bind:value={username} />
      </div>
      <div class="mb-3">
        <label class="form-label" for="login-password">Password</label>
        <input id="login-password" class="form-control" type="password" bind:value={password} />
      </div>
      <button class="btn btn-warning text-dark w-100" on:click={submit}>Sign In</button>
    </div>
  </div>
</div>
