<script lang="ts">
  export let compliant = false
  export let issues: string[] | null = []

  $: normalizedIssues = issues ?? []
</script>

<span class={`badge ${compliant ? 'bg-success' : 'bg-danger'}`} title={normalizedIssues.join(', ')}>
  {compliant ? 'Compliant' : `${normalizedIssues.length} issues`}
</span>
