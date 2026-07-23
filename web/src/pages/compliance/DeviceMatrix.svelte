<!--
  Right-column device-by-compliance view of the Compliance page: shows
  the compliant + non-compliant totals plus the per-device status table.
  Recomputes compliantDevices / nonCompliantDevices reactively from the
  shared $devices store so a refresh on any page rolls through here too.

  Extracted from Compliance.svelte in v0.3.0 (M2 — Block 4b.3 of
  docs/plans/phase-4b-refactor-block.md). Parent passes `loading` so the
  table can show its skeleton state during the initial fetch; everything
  else flows from the $devices store.
-->
<script lang="ts">
  import { devices } from '../../lib/stores';
  import type { Device } from '../../lib/types';
  import ComplianceBadge from '../../components/ComplianceBadge.svelte';

  export let loading: boolean;

  $: compliantDevices = $devices.filter((device: Device) => device.compliant);
  $: nonCompliantDevices = $devices.filter((device: Device) => !device.compliant);
</script>

<div class="card bg-dark border-info">
  <div class="card-body">
    <h2 class="h6">Summary</h2>
    <p class="mb-2">
      <span class="badge bg-success me-2">{compliantDevices.length}</span> compliant
    </p>
    <p class="mb-2">
      <span class="badge bg-danger me-2">{nonCompliantDevices.length}</span> non-compliant
    </p>
    <p class="text-secondary mb-2">
      Token <code class="font-monospace">{'{device_name}'}</code> is substituted during provisioning.
    </p>
  </div>
</div>

<div class="card bg-dark border-secondary mt-3">
  <div class="card-body">
    <h2 class="h5">Device Compliance</h2>
    {#if loading}
      <div class="text-secondary">Loading device statuses...</div>
    {:else if $devices.length === 0}
      <div class="alert alert-secondary mb-0">No enrolled devices available yet.</div>
    {:else}
      <div class="table-responsive device-list-scroll">
        <table class="table table-dark table-striped table-nowrap mb-0">
          <thead>
            <tr>
              <th>Device</th>
              <th>IP</th>
              <th>Gen</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {#each $devices as device (device.mac)}
              <tr>
                <td>{device.name || device.serial || device.mac}</td>
                <td>{device.ip}</td>
                <td>Gen{device.gen}</td>
                <td
                  ><ComplianceBadge
                    compliant={device.compliant}
                    issues={device.compliance_issues}
                  /></td
                >
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</div>
