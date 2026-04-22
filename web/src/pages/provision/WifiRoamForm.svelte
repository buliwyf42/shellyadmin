<script lang="ts">
  import type { WifiRoamState } from './types';
  import FieldRow from '../../components/FieldRow.svelte';

  export let value: WifiRoamState;
  export let disabled: boolean = false;
</script>

<div class="sa-wifi-sta">
  <div class="sa-wifi-sta-heading">Roaming</div>
  <div class="sa-form-grid">
    <div data-span="4">
      <FieldRow
        label="RSSI Threshold (dBm)"
        bind:enabled={value.rssiThrEnabled}
        {disabled}
        help="Signal below this triggers scan (default −80)"
      >
        <input
          class="form-control"
          type="number"
          bind:value={value.rssiThr}
          disabled={disabled || !value.rssiThrEnabled}
        />
      </FieldRow>
    </div>
    <div data-span="4">
      <FieldRow
        label="Interval (s)"
        bind:enabled={value.intervalEnabled}
        {disabled}
        help="Scan interval in seconds; 0 disables periodic scan"
      >
        <input
          class="form-control"
          type="number"
          min="0"
          bind:value={value.interval}
          disabled={disabled || !value.intervalEnabled}
        />
      </FieldRow>
    </div>
  </div>
</div>

<style>
  .sa-wifi-sta {
    margin-top: var(--space-3);
    padding-top: var(--space-3);
    border-top: 1px solid var(--border-soft);
  }
  .sa-wifi-sta-heading {
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--muted);
    margin-bottom: var(--space-3);
  }
</style>
