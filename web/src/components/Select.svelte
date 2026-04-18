<script lang="ts" generics="T extends string | number | boolean | undefined">
  import { onDestroy, tick } from 'svelte'

  export let value: T
  export let options: Array<{ value: NonNullable<T>; label: string; description?: string }>
  export let disabled: boolean = false
  export let placeholder: string = ''
  export let ariaLabel: string = ''
  export let width: string = ''

  let open = false
  let rootEl: HTMLDivElement | null = null
  let listEl: HTMLDivElement | null = null
  let buttonEl: HTMLButtonElement | null = null
  let highlight = -1

  $: selectedOption = options.find((o) => o.value === value)
  $: displayLabel = selectedOption ? selectedOption.label : placeholder

  function onOutside(event: MouseEvent) {
    if (!open || !rootEl) return
    const target = event.target as Node
    if (!rootEl.contains(target)) close()
  }

  async function toggleOpen() {
    if (disabled) return
    if (open) {
      close()
      return
    }
    open = true
    highlight = Math.max(0, options.findIndex((o) => o.value === value))
    await tick()
    listEl?.querySelector<HTMLElement>('.sa-select-option.is-highlight')?.scrollIntoView({ block: 'nearest' })
    document.addEventListener('mousedown', onOutside)
  }

  function close() {
    open = false
    document.removeEventListener('mousedown', onOutside)
    buttonEl?.focus()
  }

  function choose(index: number) {
    const option = options[index]
    if (!option) return
    value = option.value
    close()
  }

  function onKey(event: KeyboardEvent) {
    if (disabled) return
    if (!open) {
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp' || event.key === 'Enter' || event.key === ' ') {
        event.preventDefault()
        toggleOpen()
      }
      return
    }
    if (event.key === 'Escape') {
      event.preventDefault()
      close()
      return
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      highlight = (highlight + 1) % options.length
      return
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      highlight = (highlight - 1 + options.length) % options.length
      return
    }
    if (event.key === 'Home') {
      event.preventDefault()
      highlight = 0
      return
    }
    if (event.key === 'End') {
      event.preventDefault()
      highlight = options.length - 1
      return
    }
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      choose(highlight)
    }
  }

  onDestroy(() => document.removeEventListener('mousedown', onOutside))
</script>

<div class="sa-select" bind:this={rootEl} class:is-open={open} class:is-disabled={disabled} style={width ? `width:${width}` : ''}>
  <button
    type="button"
    class="sa-select-trigger"
    bind:this={buttonEl}
    on:click={toggleOpen}
    on:keydown={onKey}
    aria-haspopup="listbox"
    aria-expanded={open}
    aria-label={ariaLabel}
    {disabled}
  >
    <span class="sa-select-value" class:is-placeholder={!selectedOption}>{displayLabel}</span>
    <span class="sa-select-caret" aria-hidden="true"></span>
  </button>
  {#if open}
    <div class="sa-select-panel" role="listbox" bind:this={listEl}>
      {#each options as option, idx}
        <button
          type="button"
          class="sa-select-option"
          class:is-selected={option.value === value}
          class:is-highlight={idx === highlight}
          role="option"
          aria-selected={option.value === value}
          on:mouseenter={() => (highlight = idx)}
          on:click={() => choose(idx)}
        >
          <span class="sa-select-option-label">{option.label}</span>
          {#if option.description}
            <span class="sa-select-option-desc">{option.description}</span>
          {/if}
        </button>
      {/each}
    </div>
  {/if}
</div>
