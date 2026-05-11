<!--
  Template-loader / save / rename / delete toolbar for the Provision page,
  plus the credential dropdown that drives the per-template auth and the
  Form/JSON view switch (advanced-mode only).

  Extracted from Provision.svelte in v0.3.0 (M2 — Block 4b.3 of
  docs/plans/phase-4b-refactor-block.md). The parent keeps every
  template-action handler (load / save / delete / rename / setView)
  because they mutate the per-section state objects (SysState, MqttState,
  ...) that live on the parent; the panel routes each button to the
  matching callback.

  Two-way binds: selectedTemplate, templateName, selectedTemplateCredentialRef
  flow back via bind so the parent doesn't have to plumb separate input
  events.
-->
<script lang="ts">
  import Select from '../../components/Select.svelte';

  type SelectOption = { value: string; label: string; disabled?: boolean };

  export let selectedTemplate: string;
  export let templateOptions: SelectOption[];
  export let templateName: string;
  export let selectedTemplateCredentialRef: string;
  export let credentialOptions: SelectOption[];
  export let advancedModeEnabled: boolean;
  export let viewMode: 'form' | 'json';
  export let groupCredentialHint: string;
  export let onLoad: () => void;
  export let onDelete: () => void;
  export let onSave: () => void;
  export let onRename: () => void;
  export let onSetView: (mode: 'form' | 'json') => void;
</script>

<div class="card-header">
  <div class="provision-toolbar">
    <div class="sa-cluster">
      <div>
        <span class="sa-cluster-label">Template</span>
        <div class="sa-cluster-inner">
          <Select
            bind:value={selectedTemplate}
            options={templateOptions}
            placeholder="Select a template…"
            ariaLabel="Load template"
          />
          <button
            class="btn btn-sm btn-outline-light"
            on:click={onLoad}
            disabled={!selectedTemplate}>Load</button
          >
          <button
            class="btn btn-sm btn-outline-danger"
            on:click={onDelete}
            disabled={!selectedTemplate}>Delete</button
          >
        </div>
      </div>
    </div>
    <div class="sa-cluster">
      <div>
        <span class="sa-cluster-label">Save as</span>
        <div class="sa-cluster-inner">
          <input class="form-control" placeholder="template name" bind:value={templateName} />
          <button class="btn btn-sm btn-outline-light" on:click={onSave}>Save</button>
          <button
            class="btn btn-sm btn-outline-secondary"
            on:click={onRename}
            disabled={!selectedTemplate ||
              !templateName.trim() ||
              selectedTemplate === templateName.trim()}>Rename</button
          >
        </div>
      </div>
    </div>
    <div class="sa-cluster">
      <div>
        <span class="sa-cluster-label">Credential</span>
        <div class="sa-cluster-inner">
          <Select
            bind:value={selectedTemplateCredentialRef}
            options={credentialOptions}
            placeholder="No credential"
            ariaLabel="Credential"
          />
        </div>
      </div>
    </div>
    {#if advancedModeEnabled}
      <div class="sa-cluster-spacer"></div>
      <div class="sa-view-switch" role="group" aria-label="View mode">
        <button
          type="button"
          class:is-active={viewMode === 'form'}
          on:click={() => onSetView('form')}>Form</button
        >
        <button
          type="button"
          class:is-active={viewMode === 'json'}
          on:click={() => onSetView('json')}>JSON</button
        >
      </div>
    {/if}
  </div>
  {#if groupCredentialHint}
    <div class="text-secondary mt-2 text-hint-md">{groupCredentialHint}</div>
  {/if}
</div>
