<script lang="ts">
  export let compliant = false;
  export let issues: string[] | null = [];

  $: normalizedIssues = issues ?? [];
  $: hoverText = compliant
    ? 'Compliant'
    : normalizedIssues.length > 0
      ? `Non-compliant: ${normalizedIssues.join(', ')}`
      : 'Non-compliant';
</script>

<span class={`badge ${compliant ? 'bg-success' : 'bg-danger'}`} title={hoverText}>
  {compliant ? 'Compliant' : `${normalizedIssues.length} issues`}
</span>
