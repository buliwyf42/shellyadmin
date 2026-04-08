<script lang="ts">
  export let compliant = false
  export let issues: string[] = []
</script>

<span class={`badge ${compliant ? 'bg-success' : 'bg-danger'}`} title={issues.join(', ')}>
  {compliant ? 'Compliant' : `${issues.length} issues`}
</span>
