<script lang="ts">
  import { deriveSortHeaderState, type SortDir } from './sortHeader';

  export let label: string;
  export let column: string;
  export let sortKey: string;
  export let sortDir: SortDir;
  export let onSort: (key: string) => void;

  $: ({ ariaSort, nextDir, indicator } = deriveSortHeaderState(sortKey, column, sortDir));
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
