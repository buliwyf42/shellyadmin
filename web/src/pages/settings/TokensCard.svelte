<!--
  Personal Access Tokens management card for the Settings page.
  Mints + lists + revokes machine-credential bearer tokens against
  /api/tokens. Three view states:

    1. list   — the table of existing PATs with revoke buttons.
    2. create — the new-token form (name + scope checkboxes + expiry).
    3. minted — one-time display of the plaintext bearer string;
                copy-to-clipboard + "Done" return to list view.

  T3 in v0.3.0 (docs/plans/phase-4c-auth-strategics.md, Block 4c.2).

  Cookie-only: PAT-authed callers cannot mint or revoke other PATs (the
  handler enforces this with a 403). The card itself doesn't need to
  know — it always runs inside the cookie-authed Settings page.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { APIError, api } from '../../lib/api';
  import type { CreateTokenResponse, ListedPAT } from '../../lib/types';

  let tokens: ListedPAT[] = [];
  let availableScopes: string[] = [];
  let view: 'list' | 'create' | 'minted' = 'list';

  // Create-form state.
  let newName = '';
  let newScopes: Record<string, boolean> = {};
  let newExpiresInDays = 90;
  let minted: CreateTokenResponse | null = null;

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

  async function loadTokens() {
    error = '';
    try {
      const res = await api.tokens.list();
      tokens = res.tokens ?? [];
      availableScopes = res.available_scopes ?? [];
      // Initialise the scope checkbox state every reload so the
      // create form is consistent with the catalog the server
      // reports.
      const next: Record<string, boolean> = {};
      for (const s of availableScopes) next[s] = newScopes[s] ?? false;
      newScopes = next;
    } catch (err) {
      captureError(err);
    }
  }

  function startCreate() {
    error = '';
    newName = '';
    newExpiresInDays = 90;
    const next: Record<string, boolean> = {};
    for (const s of availableScopes) next[s] = false;
    newScopes = next;
    view = 'create';
  }

  async function submitCreate() {
    error = '';
    const trimmed = newName.trim();
    if (!trimmed) {
      error = 'Name is required';
      return;
    }
    const scopes = availableScopes.filter((s) => newScopes[s]);
    if (scopes.length === 0) {
      error = 'Pick at least one scope';
      return;
    }
    busy = true;
    try {
      minted = await api.tokens.create(trimmed, scopes, newExpiresInDays);
      view = 'minted';
      await loadTokens();
    } catch (err) {
      captureError(err);
    } finally {
      busy = false;
    }
  }

  function cancelCreate() {
    error = '';
    view = 'list';
  }

  function finishMinted() {
    minted = null;
    view = 'list';
  }

  async function revokeToken(id: string, name: string) {
    if (!confirm(`Revoke token "${name}"? This cannot be undone.`)) {
      return;
    }
    busy = true;
    try {
      await api.tokens.revoke(id);
      flashStatus(`Token "${name}" revoked`);
      await loadTokens();
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

  function statusBadge(t: ListedPAT): { cls: string; label: string } {
    if (t.revoked) return { cls: 'bg-secondary', label: 'Revoked' };
    if (t.expired) return { cls: 'bg-warning text-dark', label: 'Expired' };
    return { cls: 'bg-success', label: 'Active' };
  }

  function formatRelativeOrEmpty(iso?: string): string {
    if (!iso) return '—';
    // The backend already emits RFC3339; render verbatim. A relative-
    // time formatter would be a nice-to-have but adds a dep.
    return iso;
  }

  onMount(() => void loadTokens());
</script>

<div class="card bg-dark border-secondary h-100">
  <div class="card-body">
    <h2 class="h5 d-flex align-items-center gap-2">
      Personal Access Tokens
      <span class="badge bg-secondary">{tokens.length}</span>
    </h2>
    <p class="text-secondary small mb-3">
      Bearer-token credentials for headless callers — Home Assistant, cron jobs, scripts. The
      plaintext token is shown exactly once when you create it; store it somewhere safe (a password
      manager works). Use as
      <code>Authorization: Bearer pat_…</code>; CSRF is not required when calling with a bearer
      token. Each token's scopes gate which endpoints it can reach.
    </p>

    {#if error}
      <div class="alert alert-danger py-2 small">{error}</div>
    {/if}
    {#if statusMessage}
      <div class="alert alert-secondary py-2 small">{statusMessage}</div>
    {/if}

    {#if view === 'minted' && minted}
      <h3 class="h6">Token created — copy it now</h3>
      <p class="text-secondary small mb-2">
        This is the only time the plaintext token will be shown. Copy it into your client config
        before clicking Done.
      </p>
      <div class="input-group mb-3">
        <input
          class="form-control font-monospace"
          readonly
          value={minted.token}
          aria-label="Plaintext bearer token"
        />
        <button
          class="btn btn-outline-light"
          type="button"
          on:click={() => copyToClipboard(minted!.token, 'Token')}
        >
          Copy
        </button>
      </div>
      <dl class="row small mb-3">
        <dt class="col-sm-4 text-secondary">ID</dt>
        <dd class="col-sm-8 font-monospace">{minted.id}</dd>
        <dt class="col-sm-4 text-secondary">Name</dt>
        <dd class="col-sm-8">{minted.name}</dd>
        <dt class="col-sm-4 text-secondary">Scopes</dt>
        <dd class="col-sm-8">
          {#each minted.scopes as scope (scope)}
            <span class="badge bg-info text-dark me-1">{scope}</span>
          {/each}
        </dd>
        <dt class="col-sm-4 text-secondary">Expires</dt>
        <dd class="col-sm-8 font-monospace">{minted.expires_at || 'Never'}</dd>
      </dl>
      <button class="btn btn-warning text-dark" on:click={finishMinted}>Done</button>
    {:else if view === 'create'}
      <h3 class="h6">New token</h3>
      <div class="mb-3">
        <label class="form-label" for="token-name">Name</label>
        <input
          id="token-name"
          class="form-control"
          placeholder="e.g. home-assistant-bridge"
          bind:value={newName}
        />
      </div>
      <fieldset class="mb-3">
        <legend class="form-label">Scopes</legend>
        {#each availableScopes as scope (scope)}
          <label class="d-flex gap-2 align-items-center mb-1">
            <input type="checkbox" class="form-check-input" bind:checked={newScopes[scope]} />
            <code class="small">{scope}</code>
          </label>
        {/each}
      </fieldset>
      <div class="mb-3">
        <label class="form-label" for="token-expires">Expires in (days)</label>
        <input
          id="token-expires"
          class="form-control"
          type="number"
          min="0"
          max="1825"
          bind:value={newExpiresInDays}
        />
        <small class="text-secondary">0 = never expires.</small>
      </div>
      <div class="d-flex gap-2">
        <button class="btn btn-warning text-dark" on:click={submitCreate} disabled={busy}>
          Create Token
        </button>
        <button class="btn btn-outline-light" on:click={cancelCreate} disabled={busy}>
          Cancel
        </button>
      </div>
    {:else}
      {#if tokens.length === 0}
        <p class="text-secondary small">
          No tokens yet. Create one to authenticate a headless caller.
        </p>
      {:else}
        <div class="table-responsive mb-3">
          <table class="table table-dark table-sm align-middle">
            <thead>
              <tr>
                <th scope="col">Name</th>
                <th scope="col">Scopes</th>
                <th scope="col">Status</th>
                <th scope="col">Last used</th>
                <th scope="col">Expires</th>
                <th scope="col" class="text-end">Actions</th>
              </tr>
            </thead>
            <tbody>
              {#each tokens as t (t.id)}
                {@const badge = statusBadge(t)}
                <tr>
                  <td>
                    <div>{t.name}</div>
                    <div class="text-secondary small font-monospace">{t.id}</div>
                  </td>
                  <td>
                    {#each t.scopes as scope (scope)}
                      <span class="badge bg-info text-dark me-1">{scope}</span>
                    {/each}
                  </td>
                  <td>
                    <span class="badge {badge.cls}">{badge.label}</span>
                  </td>
                  <td class="font-monospace small">{formatRelativeOrEmpty(t.last_used_at)}</td>
                  <td class="font-monospace small">{t.expires_at || 'Never'}</td>
                  <td class="text-end">
                    {#if !t.revoked}
                      <button
                        class="btn btn-sm btn-outline-danger"
                        on:click={() => revokeToken(t.id, t.name)}
                        disabled={busy}
                      >
                        Revoke
                      </button>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
      <button class="btn btn-warning text-dark" on:click={startCreate}>+ New Token</button>
    {/if}
  </div>
</div>
