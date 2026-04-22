<script lang="ts">
  import type { ScriptsState } from './types';
  import SectionCard from '../../components/SectionCard.svelte';
  import Toggle from '../../components/Toggle.svelte';

  export let state: ScriptsState;

  $: expanded = state.enabled || state.scripts.length > 0;

  function addScript() {
    const nextId =
      state.scripts.length > 0
        ? String(Math.max(...state.scripts.map((s) => parseInt(s.id) || 0)) + 1)
        : '1';
    state.scripts = [...state.scripts, { id: nextId, name: '', enable: true }];
    state.enabled = true;
  }

  function removeScript(index: number) {
    state.scripts = state.scripts.filter((_, i) => i !== index);
    if (state.scripts.length === 0) state.enabled = false;
  }
</script>

<SectionCard
  tag="script"
  title="Scripts"
  bind:open={state.open}
  forceOpen={expanded}
  bind:enabled={state.enabled}
>
  <div class="sa-scripts-notice">
    Script source code (Script.PutCode) is not yet supported — configure name and enable state only.
  </div>

  {#if state.scripts.length > 0}
    <div class="sa-scripts-table-wrap">
      <table class="sa-scripts-table">
        <thead>
          <tr>
            <th>ID</th>
            <th>Name</th>
            <th>Enable</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each state.scripts as entry, i}
            <tr>
              <td>
                <input
                  class="form-control form-control-sm sa-scripts-id"
                  bind:value={entry.id}
                  disabled={!state.enabled}
                  aria-label="Script ID"
                />
              </td>
              <td>
                <input
                  class="form-control form-control-sm"
                  bind:value={entry.name}
                  disabled={!state.enabled}
                  placeholder="script name"
                  aria-label="Script name"
                />
              </td>
              <td class="sa-scripts-toggle-cell">
                <Toggle
                  bind:checked={entry.enable}
                  disabled={!state.enabled}
                  label={entry.enable ? 'On' : 'Off'}
                />
              </td>
              <td>
                <button
                  class="btn btn-sm btn-outline-danger"
                  on:click={() => removeScript(i)}
                  disabled={!state.enabled}
                  aria-label="Remove script">×</button
                >
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  <button class="btn btn-sm btn-outline-light mt-2" on:click={addScript}>Add Script</button>
</SectionCard>

<style>
  .sa-scripts-notice {
    font-size: 0.8rem;
    color: var(--muted);
    margin-bottom: var(--space-3);
  }
  .sa-scripts-table-wrap {
    overflow-x: auto;
  }
  .sa-scripts-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.85rem;
  }
  .sa-scripts-table th,
  .sa-scripts-table td {
    padding: var(--space-1) var(--space-2);
    vertical-align: middle;
  }
  .sa-scripts-table th {
    color: var(--muted);
    font-weight: 600;
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .sa-scripts-id {
    width: 5rem;
  }
  .sa-scripts-toggle-cell {
    text-align: center;
  }
</style>
