<script lang="ts">
  export let enabled: boolean;
  export let label: string;
  export let disabled: boolean = false;
  export let help: string = '';
</script>

<div class="sa-field" class:is-disabled={disabled} class:is-off={!enabled}>
  <label class="sa-field-head">
    <span class="sa-check" class:is-checked={enabled} class:is-disabled={disabled}>
      <input type="checkbox" bind:checked={enabled} {disabled} />
      <svg viewBox="0 0 12 12" aria-hidden="true" class="sa-check-mark">
        <path
          d="M2 6.4 L4.8 9 L10 3.2"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
      </svg>
    </span>
    <span class="sa-field-label">{label}</span>
  </label>
  <div class="sa-field-control">
    <slot />
  </div>
  {#if help}
    <div class="sa-field-help">{help}</div>
  {/if}
</div>
