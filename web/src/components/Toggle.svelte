<script lang="ts">
  export let checked: boolean | null | undefined = false;
  export let disabled: boolean = false;
  export let label: string = '';
  export let ariaLabel: string = '';

  $: isOn = checked === true;

  function onKey(e: KeyboardEvent) {
    if (disabled) return;
    if (e.key === ' ' || e.key === 'Enter') {
      e.preventDefault();
      checked = !isOn;
    }
  }
</script>

<label class="sa-toggle" class:is-disabled={disabled}>
  <span
    class="sa-toggle-track"
    class:is-on={isOn}
    role="switch"
    tabindex={disabled ? -1 : 0}
    aria-checked={isOn}
    aria-label={ariaLabel || label}
    aria-disabled={disabled}
    on:keydown={onKey}
  >
    <input type="checkbox" bind:checked {disabled} class="sa-toggle-input" />
    <span class="sa-toggle-knob"></span>
  </span>
  {#if label || $$slots.default}
    <span class="sa-toggle-label"
      >{#if label}{label}{:else}<slot />{/if}</span
    >
  {/if}
</label>
