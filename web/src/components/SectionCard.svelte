<script lang="ts">
  export let tag: string = '';
  export let title: string = '';
  export let open: boolean;
  export let forceOpen: boolean = false;
  export let enabled: boolean | null = null;
  export let enableDisabled: boolean = false;

  $: visible = forceOpen || open;
  $: hasEnable = enabled !== null;

  function toggleOpen() {
    open = !open;
  }

  function onHeadKey(e: KeyboardEvent) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      toggleOpen();
    }
  }

  function onEnableClick(e: Event) {
    e.stopPropagation();
  }

  function onEnableKey(e: KeyboardEvent) {
    if (e.key === ' ' || e.key === 'Enter') {
      e.stopPropagation();
    }
  }
</script>

<section class="sa-section" class:is-open={visible} class:is-active={hasEnable && enabled}>
  <div
    class="sa-section-head"
    role="button"
    tabindex="0"
    on:click={toggleOpen}
    on:keydown={onHeadKey}
    aria-expanded={visible}
  >
    {#if hasEnable}
      <span class="sa-section-enable">
        <span class="sa-check" class:is-checked={enabled} class:is-disabled={enableDisabled}>
          <input
            type="checkbox"
            bind:checked={enabled}
            disabled={enableDisabled}
            aria-label={title ? `Enable ${title}` : 'Enable section'}
            on:click={onEnableClick}
            on:keydown={onEnableKey}
          />
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
      </span>
    {/if}
    <div class="sa-section-title">
      {#if tag}<strong class="sa-section-tag">{tag}</strong>{/if}
      {#if title}<span class="sa-section-name">{title}</span>{/if}
    </div>
    <span class="sa-section-caret" class:is-open={visible} aria-hidden="true"></span>
  </div>
  {#if visible}
    <div class="sa-section-body">
      <slot />
    </div>
  {/if}
</section>
