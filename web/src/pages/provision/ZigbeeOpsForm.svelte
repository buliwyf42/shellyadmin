<script lang="ts">
  import type { ZigbeeOpsState } from './types';
  import SectionCard from '../../components/SectionCard.svelte';
  import FieldRow from '../../components/FieldRow.svelte';

  export let state: ZigbeeOpsState;

  $: anyEnabled = state.sendCommandEnabled || state.readAttrEnabled || state.writeAttrEnabled;
</script>

<SectionCard
  tag="zigbee_ops"
  title="Zigbee operations (advanced)"
  bind:open={state.open}
  forceOpen={anyEnabled}
>
  <div class="sa-zigbee-notice">
    Builds a <code>gen2_rpc</code> template section with one or more direct ZCL operations against
    paired Zigbee devices. Targets devices that expose the
    <code>Zigbee.*</code> RPC namespace (Shelly Wave gateways and similar). Each operation requires
    the device's <code>eui64</code> (64-bit address). <strong>Write-mostly:</strong>
    saved templates with <code>gen2_rpc</code> sections do not load back into the form view — use JSON
    view to inspect / edit afterwards.
  </div>

  <div class="sa-zigbee-card">
    <FieldRow label="Zigbee.SendCommand" bind:enabled={state.sendCommandEnabled}>
      <span class="text-secondary" style="font-size: 0.78rem;">
        send a ZCL command (cluster + cmd + optional hex payload)
      </span>
    </FieldRow>
    {#if state.sendCommandEnabled}
      <div class="sa-form-grid sa-zigbee-grid">
        <div data-span="6">
          <label class="form-label" for="sa-zb-send-eui64">eui64</label>
          <input
            id="sa-zb-send-eui64"
            class="form-control font-monospace"
            bind:value={state.sendCommand.eui64}
            placeholder="0x00158d0001abcd1234"
          />
        </div>
        <div data-span="2">
          <label class="form-label" for="sa-zb-send-ep">endpoint</label>
          <input
            id="sa-zb-send-ep"
            class="form-control"
            type="number"
            min="0"
            bind:value={state.sendCommand.ep}
          />
        </div>
        <div data-span="2">
          <label class="form-label" for="sa-zb-send-cluster">cluster</label>
          <input
            id="sa-zb-send-cluster"
            class="form-control"
            type="number"
            min="0"
            bind:value={state.sendCommand.cluster}
          />
        </div>
        <div data-span="2">
          <label class="form-label" for="sa-zb-send-cmd">cmd</label>
          <input
            id="sa-zb-send-cmd"
            class="form-control"
            type="number"
            min="0"
            bind:value={state.sendCommand.cmd}
          />
        </div>
        <div data-span="12">
          <label class="form-label" for="sa-zb-send-payload">payload (hex, optional)</label>
          <input
            id="sa-zb-send-payload"
            class="form-control font-monospace"
            bind:value={state.sendCommand.payload}
            placeholder="e.g. 0102030a"
          />
        </div>
      </div>
    {/if}
  </div>

  <div class="sa-zigbee-card">
    <FieldRow label="Zigbee.ReadAttr" bind:enabled={state.readAttrEnabled}>
      <span class="text-secondary" style="font-size: 0.78rem;">
        read attributes by id from a cluster
      </span>
    </FieldRow>
    {#if state.readAttrEnabled}
      <div class="sa-form-grid sa-zigbee-grid">
        <div data-span="6">
          <label class="form-label" for="sa-zb-read-eui64">eui64</label>
          <input
            id="sa-zb-read-eui64"
            class="form-control font-monospace"
            bind:value={state.readAttr.eui64}
            placeholder="0x00158d0001abcd1234"
          />
        </div>
        <div data-span="2">
          <label class="form-label" for="sa-zb-read-ep">endpoint</label>
          <input
            id="sa-zb-read-ep"
            class="form-control"
            type="number"
            min="0"
            bind:value={state.readAttr.ep}
          />
        </div>
        <div data-span="2">
          <label class="form-label" for="sa-zb-read-cluster">cluster</label>
          <input
            id="sa-zb-read-cluster"
            class="form-control"
            type="number"
            min="0"
            bind:value={state.readAttr.cluster}
          />
        </div>
        <div data-span="12">
          <label class="form-label" for="sa-zb-read-attrs">
            attribute ids (comma- or space-separated)
          </label>
          <input
            id="sa-zb-read-attrs"
            class="form-control"
            bind:value={state.readAttr.attrs}
            placeholder="e.g. 0, 4, 5, 1024"
          />
        </div>
      </div>
    {/if}
  </div>

  <div class="sa-zigbee-card">
    <FieldRow label="Zigbee.WriteAttr" bind:enabled={state.writeAttrEnabled}>
      <span class="text-secondary" style="font-size: 0.78rem;">
        write a list of {`{id, type, value}`} attribute records (raw JSON)
      </span>
    </FieldRow>
    {#if state.writeAttrEnabled}
      <div class="sa-form-grid sa-zigbee-grid">
        <div data-span="6">
          <label class="form-label" for="sa-zb-write-eui64">eui64</label>
          <input
            id="sa-zb-write-eui64"
            class="form-control font-monospace"
            bind:value={state.writeAttr.eui64}
            placeholder="0x00158d0001abcd1234"
          />
        </div>
        <div data-span="2">
          <label class="form-label" for="sa-zb-write-ep">endpoint</label>
          <input
            id="sa-zb-write-ep"
            class="form-control"
            type="number"
            min="0"
            bind:value={state.writeAttr.ep}
          />
        </div>
        <div data-span="2">
          <label class="form-label" for="sa-zb-write-cluster">cluster</label>
          <input
            id="sa-zb-write-cluster"
            class="form-control"
            type="number"
            min="0"
            bind:value={state.writeAttr.cluster}
          />
        </div>
        <div data-span="12">
          <label class="form-label" for="sa-zb-write-attrs">attrs (JSON array)</label>
          <textarea
            id="sa-zb-write-attrs"
            class="form-control font-monospace"
            rows="4"
            bind:value={state.writeAttr.attrsJSON}
            placeholder={'[ { "id": 0, "type": "uint8", "value": 1 } ]'}
          ></textarea>
          <div class="text-secondary" style="font-size: 0.78rem; margin-top: 0.25rem;">
            ZCL type strings like uint8, int16, bool, string. The backend forwards as-is to
            <code>Zigbee.WriteAttr</code>.
          </div>
        </div>
      </div>
    {/if}
  </div>
</SectionCard>

<style>
  .sa-zigbee-notice {
    font-size: 0.8rem;
    color: var(--muted);
    margin-bottom: var(--space-3);
  }
  .sa-zigbee-card {
    border: 1px solid var(--border);
    border-radius: var(--radius-2);
    padding: var(--space-2) var(--space-3);
    margin-bottom: var(--space-2);
  }
  .sa-zigbee-grid {
    margin-top: var(--space-2);
  }
</style>
