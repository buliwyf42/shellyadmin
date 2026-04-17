<script lang="ts">
  export let label: string
  export let column: string
  export let sortKey: string
  export let sortDir: 'asc' | 'desc'
  export let onSort: (key: string) => void

  $: active = sortKey === column
  $: ariaSort = active ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'
  $: nextDir = active && sortDir === 'asc' ? 'descending' : 'ascending'
  $: indicator = active ? (sortDir === 'asc' ? ' ▲' : ' ▼') : ''
</script>

<th aria-sort={ariaSort}>
  <button
    type="button"
    class="btn btn-link px-0 text-decoration-none"
    on:click={() => onSort(column)}
    aria-label={`Sort by ${label} ${nextDir}`}
  >
    {label}<span aria-hidden="true">{indicator}</span>
  </button>
</th>
