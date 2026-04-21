<script lang="ts">
  export let done: number
  export let total: number
  export let running: boolean
  export let label: string | undefined = undefined
  export let ariaLabel: string

  $: pct = total > 0 ? Math.min(100, (done / total) * 100) : 0
  $: determinate = total > 0
  $: labelInside = determinate && pct >= 25
</script>

<div
  class="pb-track"
  role="progressbar"
  aria-label={ariaLabel}
  aria-busy={running || undefined}
  aria-valuenow={determinate ? done : undefined}
  aria-valuemin={determinate ? 0 : undefined}
  aria-valuemax={determinate ? total : undefined}
>
  <div
    class="pb-fill"
    class:pb-running={running}
    class:pb-indeterminate={running && !determinate}
    style={determinate ? `width:${pct}%` : undefined}
  >
    {#if label && labelInside}
      <span class="pb-label">{label}</span>
    {/if}
  </div>
  {#if label && !labelInside}
    <span class="pb-label-below">{label}</span>
  {/if}
</div>

<style>
  .pb-track {
    position: relative;
    background: rgba(255, 255, 255, 0.06);
    border-radius: 999px;
    overflow: hidden;
    min-height: 1.15rem;
  }

  .pb-fill {
    height: 1.15rem;
    background: linear-gradient(90deg, #d2aa48, #f0cf7a);
    border-radius: 999px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.8rem;
    font-weight: 700;
    color: #1f1805;
    transition: width 200ms ease;
    position: relative;
    overflow: hidden;
    min-width: 0;
  }

  .pb-indeterminate {
    width: 100% !important;
  }

  .pb-running::after {
    content: '';
    position: absolute;
    inset: 0;
    background: repeating-linear-gradient(
      45deg,
      rgba(255, 255, 255, 0.18) 0 10px,
      transparent 10px 20px
    );
    background-size: 40px 40px;
    animation: pb-stripes 1s linear infinite;
  }

  @keyframes pb-stripes {
    from { background-position: 0 0; }
    to   { background-position: 40px 0; }
  }

  .pb-label {
    position: relative;
    z-index: 1;
    white-space: nowrap;
  }

  .pb-label-below {
    display: block;
    margin-top: 0.25rem;
    font-size: 0.8rem;
    color: rgba(255, 255, 255, 0.6);
    text-align: center;
  }
</style>
