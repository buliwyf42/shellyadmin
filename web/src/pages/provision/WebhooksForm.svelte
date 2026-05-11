<script lang="ts">
  import type { WebhooksState } from './types';
  import SectionCard from '../../components/SectionCard.svelte';
  import Toggle from '../../components/Toggle.svelte';

  export let state: WebhooksState;

  $: hasContent = state.deleteAll || state.deleteIds.trim() !== '' || state.creates.length > 0;

  function addCreate() {
    state.creates = [...state.creates, { cid: '0', event: '', urls: '', name: '', enable: true }];
  }

  function removeCreate(index: number) {
    state.creates = state.creates.filter((_, i) => i !== index);
  }
</script>

<SectionCard tag="webhooks" title="Webhooks" bind:open={state.open} forceOpen={hasContent}>
  <div class="sa-webhooks-notice">
    Webhooks fire HTTP callbacks on device events (input toggles, switch state changes, etc.). The
    provisioner applies operations in <code>delete_all</code> → <code>delete</code> →
    <code>update</code>
    → <code>create</code> order; the form covers the wipe + create case. Per-id updates require knowing
    each device's current webhook ids — switch to JSON view for those.
  </div>

  <div class="sa-webhooks-block">
    <Toggle bind:checked={state.deleteAll} label="Delete all existing webhooks before applying" />
  </div>

  <div class="sa-webhooks-block">
    <label class="form-label" for="sa-webhooks-delete-ids">
      Delete by ID (comma- or space-separated)
    </label>
    <input
      id="sa-webhooks-delete-ids"
      class="form-control form-control-sm"
      bind:value={state.deleteIds}
      placeholder="e.g. 3, 7, 12"
    />
  </div>

  <div class="sa-webhooks-block">
    <div class="d-flex justify-content-between align-items-center mb-2">
      <strong>New webhooks</strong>
      <button class="btn btn-sm btn-outline-light" on:click={addCreate}>Add Webhook</button>
    </div>

    {#if state.creates.length === 0}
      <div class="text-secondary text-hint-lg">None — click "Add Webhook" to create one.</div>
    {/if}

    {#each state.creates as entry, i (i)}
      <div class="sa-webhooks-card">
        <div class="d-flex justify-content-between align-items-center mb-2">
          <span class="text-secondary text-hint-xs">Webhook #{i + 1}</span>
          <button
            class="btn btn-sm btn-outline-danger"
            on:click={() => removeCreate(i)}
            aria-label="Remove webhook">×</button
          >
        </div>
        <div class="sa-webhooks-row">
          <div class="sa-webhooks-field-cid">
            <label class="form-label" for={`sa-webhook-cid-${i}`}>cid</label>
            <input
              id={`sa-webhook-cid-${i}`}
              class="form-control form-control-sm"
              type="number"
              min="0"
              bind:value={entry.cid}
              aria-label="Component id"
            />
          </div>
          <div class="sa-webhooks-field-event">
            <label class="form-label" for={`sa-webhook-event-${i}`}>event</label>
            <input
              id={`sa-webhook-event-${i}`}
              class="form-control form-control-sm"
              bind:value={entry.event}
              placeholder="e.g. input.toggle_on"
            />
          </div>
          <div class="sa-webhooks-field-name">
            <label class="form-label" for={`sa-webhook-name-${i}`}>name (optional)</label>
            <input
              id={`sa-webhook-name-${i}`}
              class="form-control form-control-sm"
              bind:value={entry.name}
              placeholder="human-readable label"
            />
          </div>
          <div class="sa-webhooks-field-enable">
            <span class="form-label">enable</span>
            <Toggle bind:checked={entry.enable} label={entry.enable ? 'On' : 'Off'} />
          </div>
        </div>
        <div>
          <label class="form-label" for={`sa-webhook-urls-${i}`}>URLs (one per line)</label>
          <textarea
            id={`sa-webhook-urls-${i}`}
            class="form-control form-control-sm font-monospace"
            rows="3"
            bind:value={entry.urls}
            placeholder="https://example.com/hook"
          ></textarea>
        </div>
      </div>
    {/each}
  </div>
</SectionCard>

<style>
  .sa-webhooks-notice {
    font-size: 0.8rem;
    color: var(--muted);
    margin-bottom: var(--space-3);
  }
  .sa-webhooks-block {
    margin-bottom: var(--space-3);
  }
  .sa-webhooks-card {
    border: 1px solid var(--border);
    border-radius: var(--radius-2);
    padding: var(--space-2) var(--space-3);
    margin-bottom: var(--space-2);
  }
  .sa-webhooks-row {
    display: grid;
    grid-template-columns: 5rem 1fr 1fr auto;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }
  @media (max-width: 720px) {
    .sa-webhooks-row {
      grid-template-columns: 1fr;
    }
  }
</style>
