<!--
  Custom-rules editor section for the Compliance page. Renders the
  collapsible "custom rules" SectionCard with one row per rule, plus the
  "Add Rule" button. Each row binds to the parent's
  settings.compliance.custom_rules entries directly via property-level
  bind:value, so changes flow back to the parent without explicit events.

  Extracted from Compliance.svelte in v0.3.0 (M2 — Block 4b.3 of
  docs/plans/phase-4b-refactor-block.md).
-->
<script lang="ts">
  import SectionCard from '../../components/SectionCard.svelte';
  import Select from '../../components/Select.svelte';
  import type { CustomRule } from '../../lib/types';

  export let rules: CustomRule[];
  export let open: boolean;
  export let forceOpen: boolean;
  export let sourceOptions: Array<{ value: CustomRule['source']; label: string }>;
  export let opOptions: Array<{ value: CustomRule['op']; label: string }>;
  export let onAdd: () => void;
  export let onRemove: (index: number) => void;
</script>

<SectionCard tag="custom rules" bind:open {forceOpen}>
  <p class="text-secondary mb-2" style="font-size: 0.82rem;">
    source = <code>device | config | status</code>. Example paths:
    <code>mqtt.server</code>, <code>sys.location.tz</code>, <code>cloud.connected</code>.
  </p>
  {#each rules as rule, idx (idx)}
    <div class="sa-custom-rule">
      <div class="sa-form-grid">
        <div data-span="3">
          <input class="form-control" placeholder="Label" bind:value={rule.label} />
        </div>
        <div data-span="2">
          <Select bind:value={rule.source} options={sourceOptions} ariaLabel="Source" />
        </div>
        <div data-span="3">
          <input
            class="form-control font-monospace"
            placeholder="path.to.field"
            bind:value={rule.path}
          />
        </div>
        <div data-span="2">
          <Select bind:value={rule.op} options={opOptions} ariaLabel="Operator" />
        </div>
        <div data-span="2">
          <input
            class="form-control"
            placeholder="Expected value"
            bind:value={rule.value}
            disabled={rule.op === 'exists'}
          />
        </div>
        <div data-span="2">
          <input
            class="form-control"
            type="number"
            min="0"
            placeholder="Gen min"
            bind:value={rule.gen_min}
          />
        </div>
        <div data-span="2">
          <input
            class="form-control"
            type="number"
            min="0"
            placeholder="Gen max"
            bind:value={rule.gen_max}
          />
        </div>
        <div data-span="2">
          <button class="btn btn-sm btn-outline-danger" on:click={() => onRemove(idx)}
            >Remove</button
          >
        </div>
      </div>
    </div>
  {/each}
  <button class="btn btn-sm btn-outline-light mt-2" on:click={onAdd}>Add Rule</button>
</SectionCard>

<style>
  .sa-custom-rule {
    border: 1px solid var(--border-soft);
    border-radius: var(--radius-md);
    padding: var(--space-3);
    margin-bottom: var(--space-3);
    background: rgba(255, 255, 255, 0.012);
  }
</style>
