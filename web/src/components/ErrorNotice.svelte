<script lang="ts">
  export let summary = '';
  export let details = '';
</script>

{#if summary}
  <div class="alert alert-danger" role="alert" aria-live="assertive">
    <div>{summary}</div>
    {#if details}
      <details class="mt-2">
        <summary>Technical details</summary>
        <pre class="mb-0 mt-2">{details}</pre>
      </details>
    {/if}
  </div>
{/if}
