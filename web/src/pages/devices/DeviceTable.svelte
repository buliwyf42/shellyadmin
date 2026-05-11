<!--
  Main device table for the Devices page. Renders every column declared
  in stores.ts:deviceColumns conditional on the persisted colVis store,
  plus the per-row action buttons (detail / refresh / reboot / delete).

  Extracted from Devices.svelte in v0.3.0 (M2 — Block 4b.3 of
  docs/plans/phase-4b-refactor-block.md). Pure presentation: the heavy
  lifting (data load, sort state, busy tracking, action dispatch) stays
  on the parent and reaches us via props + callbacks. Reads colVis +
  firmwareChannel directly from the persisted stores so the SPA's "what
  columns are visible" + "which update channel is highlighted" preferences
  stay applied here without prop-drilling.
-->
<script lang="ts">
  import { colVis, firmwareChannel } from '../../lib/stores';
  import { formatDateTime, formatRelativeDateTime } from '../../lib/time';
  import type { AppSettings, Device } from '../../lib/types';
  import {
    formatCoords,
    generationLabel,
    mqttManagedByCloud,
    refreshState,
    refreshStateBadgeClass,
    refreshStateText,
    refreshStateTitle,
    statusBadgeClass,
    statusText,
    supportClass as supportClassFn,
    supportTitle,
  } from '../../lib/deviceFormatters';
  import ComplianceBadge from '../../components/ComplianceBadge.svelte';
  import SortHeader from '../../components/SortHeader.svelte';

  export let devices: Device[];
  export let sortKey: string;
  export let sortDir: 'asc' | 'desc';
  export let setSort: (key: string) => void;
  export let appSettings: AppSettings | null;
  export let rowBusy: Record<string, { refresh: boolean; remove: boolean; reboot: boolean }>;
  /** Empty-state hint shown below the table when there are 0 devices.
   * Parent decides when to show (loading=false and error=""). */
  export let showEmpty: boolean;
  export let onRefresh: (device: Device) => void;
  export let onRemove: (device: Device) => void;
  export let onReboot: (device: Device) => void;
  export let onOpenDetail: (device: Device) => void;

  function supportClass(device: Device): string {
    return supportClassFn(device, appSettings);
  }
</script>

<div class="table-responsive dashboard-table-wrap">
  <table class="table table-dark table-striped align-middle table-nowrap dashboard-table">
    <thead>
      <tr>
        {#if $colVis.device_num}<SortHeader
            label="#"
            column="device_num"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.name}<SortHeader
            label="Name"
            column="name"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.ip}<SortHeader
            label="IP"
            column="ip"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.mac}<SortHeader
            label="MAC"
            column="mac"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.gen}<SortHeader
            label="Type"
            column="gen"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.model}<SortHeader
            label="Model"
            column="model"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.fw}<SortHeader
            label="Firmware"
            column="fw"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.fw_auto_update}<SortHeader
            label="Auto-Update"
            column="fw_auto_update"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.capabilities}<th title="Component instance counts (switch / cover / light)"
            >Capabilities</th
          >{/if}
        {#if $colVis.online}<SortHeader
            label="Online"
            column="online"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.wifi_ssid}<SortHeader
            label="WiFi"
            column="wifi_ssid"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.mqtt_enabled}<SortHeader
            label="MQTT"
            column="mqtt_enabled"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.mqtt_server}<SortHeader
            label="MQTT Server"
            column="mqtt_server"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.mqtt_client_id}<SortHeader
            label="MQTT Client ID"
            column="mqtt_client_id"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.mqtt_topic_prefix}<SortHeader
            label="MQTT Topic"
            column="mqtt_topic_prefix"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.cloud_connected}<SortHeader
            label="Cloud"
            column="cloud_connected"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.ws_connected}<SortHeader
            label="WebSocket"
            column="ws_connected"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.tz}<SortHeader
            label="Timezone"
            column="tz"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.sntp_server}<SortHeader
            label="SNTP"
            column="sntp_server"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.serial}<SortHeader
            label="Serial"
            column="serial"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.matter_enabled}<SortHeader
            label="Matter"
            column="matter_enabled"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.ble_gw_enabled}<SortHeader
            label="BLE GW"
            column="ble_gw_enabled"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.coords}<SortHeader
            label="Coords"
            column="coords"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.eco_mode}<SortHeader
            label="Eco"
            column="eco_mode"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.discoverable}<SortHeader
            label="Discoverable"
            column="discoverable"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.scheme}<SortHeader
            label="Scheme"
            column="scheme"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.wifi_hostname}<SortHeader
            label="Hostname"
            column="wifi_hostname"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.wifi_channel}<SortHeader
            label="WiFi Ch"
            column="wifi_channel"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.enhanced_security}<SortHeader
            label="Enhanced Sec"
            column="enhanced_security"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.tls_cert_valid}<SortHeader
            label="TLS OK"
            column="tls_cert_valid"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.power_w}<SortHeader
            label="Power (W)"
            column="power_w"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.voltage_v}<SortHeader
            label="Voltage (V)"
            column="voltage_v"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.current_a}<SortHeader
            label="Current (A)"
            column="current_a"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.first_seen}<SortHeader
            label="First Seen"
            column="first_seen"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.last_seen}<SortHeader
            label="Last Success"
            column="last_seen"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        {#if $colVis.compliance}<SortHeader
            label="Compliance"
            column="compliance"
            {sortKey}
            {sortDir}
            onSort={setSort}
          />{/if}
        <th class="text-end">Actions</th>
      </tr>
    </thead>
    <tbody>
      {#each devices as device (device.mac)}
        <tr class:device-stale={refreshState(device) === 'stale'}>
          {#if $colVis.device_num}<td>{String(device.device_num).padStart(2, '0')}</td>{/if}
          {#if $colVis.name}<td>{device.name || device.serial || device.mac}</td>{/if}
          {#if $colVis.ip}<td
              ><a href={`http://${device.ip}`} target="_blank" rel="noreferrer" class="ip-link"
                >{device.ip}</a
              ></td
            >{/if}
          {#if $colVis.mac}<td class="font-monospace">{device.mac}</td>{/if}
          {#if $colVis.gen}<td
              ><span class={`badge ${supportClass(device)}`} title={supportTitle(device)}
                >{generationLabel(device)}</span
              ></td
            >{/if}
          {#if $colVis.model}<td>
              {#if device.app || device.model}
                {@const tooltip = [
                  device.app ? `App: ${device.app}` : '',
                  device.model ? `Model: ${device.model}` : '',
                  `Gen ${device.gen}`,
                  device.switch_count ? `Switch: ${device.switch_count}` : '',
                  device.cover_count ? `Cover: ${device.cover_count}` : '',
                  device.light_count ? `Light: ${device.light_count}` : '',
                ]
                  .filter(Boolean)
                  .join('\n')}
                <div title={tooltip}>
                  {device.app || device.model}
                </div>
              {:else}<span class="text-secondary">n/a</span>{/if}
            </td>{/if}
          {#if $colVis.fw}
            <td>
              {#if device.fw}{device.fw}{:else}<span class="text-secondary">n/a</span>{/if}
              {#if $firmwareChannel === 'beta' && device.fw_available_beta && device.fw_available_beta !== device.fw}
                <span class="badge bg-info text-dark" title="beta available">
                  ↑ {device.fw_available_beta}
                </span>
              {:else if $firmwareChannel === 'stable' && device.fw_available_stable && device.fw_available_stable !== device.fw}
                <span class="badge bg-info text-dark" title="stable available">
                  ↑ {device.fw_available_stable}
                </span>
              {/if}
            </td>
          {/if}
          {#if $colVis.fw_auto_update}
            <td>
              {#if device.fw_auto_update === 'stable'}
                <span class="badge bg-success" title="auto-update Stable">stable</span>
              {:else if device.fw_auto_update === 'beta'}
                <span class="badge bg-info text-dark" title="auto-update Beta">beta</span>
              {:else if device.fw_auto_update === 'off'}
                <span class="badge bg-secondary" title="auto-update disabled">off</span>
              {:else}
                <span class="badge bg-dark border border-secondary" title="not yet read"
                  >unknown</span
                >
              {/if}
            </td>
          {/if}
          {#if $colVis.capabilities}
            <td>
              <div class="d-flex gap-1 flex-wrap">
                {#if device.switch_count}<span
                    class="badge bg-secondary"
                    title="{device.switch_count} switch component(s)">⚡ {device.switch_count}</span
                  >{/if}
                {#if device.cover_count}<span
                    class="badge bg-secondary"
                    title="{device.cover_count} cover component(s)">⇅ {device.cover_count}</span
                  >{/if}
                {#if device.light_count}<span
                    class="badge bg-secondary"
                    title="{device.light_count} light component(s)">💡 {device.light_count}</span
                  >{/if}
                {#if !device.switch_count && !device.cover_count && !device.light_count}
                  <span class="text-secondary">—</span>
                {/if}
              </div>
            </td>
          {/if}
          {#if $colVis.online}
            <td>
              <div class="d-flex gap-2 align-items-center flex-wrap">
                <span class={`badge ${statusBadgeClass(device.online)}`}
                  >{statusText(device.online, 'Online', 'Offline')}</span
                >
                <span
                  class={`badge ${refreshStateBadgeClass(device)}`}
                  title={refreshStateTitle(device)}>{refreshStateText(device)}</span
                >
              </div>
            </td>
          {/if}
          {#if $colVis.wifi_ssid}<td
              >{#if device.wifi_ssid}{device.wifi_ssid}{:else}<span class="text-secondary">n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.mqtt_enabled}
            <td
              ><span
                class={`badge ${mqttManagedByCloud(device) ? 'bg-secondary' : statusBadgeClass(device.mqtt_enabled)}`}
                >{mqttManagedByCloud(device)
                  ? 'cloud-managed'
                  : statusText(device.mqtt_enabled)}</span
              ></td
            >
          {/if}
          {#if $colVis.mqtt_server}<td
              >{#if mqttManagedByCloud(device)}cloud-managed{:else if device.mqtt_server}{device.mqtt_server}{:else}<span
                  class="text-secondary">n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.mqtt_client_id}<td
              >{#if mqttManagedByCloud(device)}cloud-managed{:else if device.mqtt_client_id}{device.mqtt_client_id}{:else}<span
                  class="text-secondary">n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.mqtt_topic_prefix}<td
              >{#if mqttManagedByCloud(device)}cloud-managed{:else if device.mqtt_topic_prefix}{device.mqtt_topic_prefix}{:else}<span
                  class="text-secondary">n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.cloud_connected}<td
              ><span class={`badge ${statusBadgeClass(device.cloud_connected)}`}
                >{statusText(device.cloud_connected, 'Connected', 'Off')}</span
              ></td
            >{/if}
          {#if $colVis.ws_connected}<td
              ><span class={`badge ${statusBadgeClass(device.ws_connected)}`}
                >{statusText(device.ws_connected, 'Connected', 'Off')}</span
              ></td
            >{/if}
          {#if $colVis.tz}<td
              >{#if device.tz}{device.tz}{:else}<span class="text-secondary">n/a</span>{/if}</td
            >{/if}
          {#if $colVis.sntp_server}<td
              >{#if device.sntp_server}{device.sntp_server}{:else}<span class="text-secondary"
                  >n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.serial}<td class="font-monospace"
              >{#if device.serial}{device.serial}{:else}<span class="text-secondary">n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.matter_enabled}<td
              ><span class={`badge ${statusBadgeClass(device.matter_enabled)}`}
                >{statusText(device.matter_enabled)}</span
              ></td
            >{/if}
          {#if $colVis.ble_gw_enabled}<td
              ><span class={`badge ${statusBadgeClass(device.ble_gw_enabled)}`}
                >{statusText(device.ble_gw_enabled)}</span
              ></td
            >{/if}
          {#if $colVis.coords}<td
              >{#if formatCoords(device) !== 'n/a'}{formatCoords(device)}{:else}<span
                  class="text-secondary">n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.eco_mode}<td
              ><span class={`badge ${statusBadgeClass(device.eco_mode)}`}
                >{statusText(device.eco_mode)}</span
              ></td
            >{/if}
          {#if $colVis.discoverable}<td
              ><span class={`badge ${statusBadgeClass(device.discoverable)}`}
                >{statusText(device.discoverable)}</span
              ></td
            >{/if}
          {#if $colVis.scheme}<td class="font-monospace">{device.scheme || 'http'}</td>{/if}
          {#if $colVis.wifi_hostname}<td
              >{#if device.wifi_hostname}{device.wifi_hostname}{:else}<span class="text-secondary"
                  >n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.wifi_channel}<td class="text-end"
              >{#if device.wifi_channel}{device.wifi_channel}{:else}<span class="text-secondary"
                  >n/a</span
                >{/if}</td
            >{/if}
          {#if $colVis.enhanced_security}<td
              ><span class={`badge ${statusBadgeClass(device.enhanced_security ?? null)}`}
                >{statusText(device.enhanced_security ?? null)}</span
              ></td
            >{/if}
          {#if $colVis.tls_cert_valid}<td
              ><span class={`badge ${statusBadgeClass(device.tls_cert_valid ?? null)}`}
                >{statusText(device.tls_cert_valid ?? null)}</span
              ></td
            >{/if}
          {#if $colVis.power_w}<td class="text-end font-monospace"
              >{#if device.power_w !== null && device.power_w !== undefined}{device.power_w.toFixed(
                  1,
                )}{:else}<span class="text-secondary">n/a</span>{/if}</td
            >{/if}
          {#if $colVis.voltage_v}<td class="text-end font-monospace"
              >{#if device.voltage_v !== null && device.voltage_v !== undefined}{device.voltage_v.toFixed(
                  0,
                )}{:else}<span class="text-secondary">n/a</span>{/if}</td
            >{/if}
          {#if $colVis.current_a}<td class="text-end font-monospace"
              >{#if device.current_a !== null && device.current_a !== undefined}{device.current_a.toFixed(
                  2,
                )}{:else}<span class="text-secondary">n/a</span>{/if}</td
            >{/if}
          {#if $colVis.first_seen}<td title={formatDateTime(device.first_seen)}
              >{formatRelativeDateTime(device.first_seen)}</td
            >{/if}
          {#if $colVis.last_seen}
            <td title={refreshStateTitle(device)}>
              {#if device.last_seen}
                {formatRelativeDateTime(device.last_seen)}
              {:else}
                <span class="text-secondary">never</span>
              {/if}
            </td>
          {/if}
          {#if $colVis.compliance}<td
              ><ComplianceBadge
                compliant={device.compliant}
                issues={device.compliance_issues}
              /></td
            >{/if}
          <td class="text-end">
            <div class="d-flex justify-content-end gap-2">
              <button
                class="btn btn-sm btn-outline-light row-action-btn"
                title="Open device detail"
                aria-label={`Open detail for ${device.name || device.mac}`}
                on:click={() => onOpenDetail(device)}
                disabled={rowBusy[device.mac]?.refresh || rowBusy[device.mac]?.remove}
                ><span aria-hidden="true">⋯</span></button
              >
              <button
                class="btn btn-sm btn-outline-light row-action-btn"
                title="Refresh this device now"
                aria-label={`Refresh ${device.name || device.mac}`}
                on:click={() => onRefresh(device)}
                disabled={rowBusy[device.mac]?.refresh || rowBusy[device.mac]?.remove}
                ><span aria-hidden="true">↻</span></button
              >
              <button
                class="btn btn-sm btn-outline-warning row-action-btn"
                title="Reboot this device"
                aria-label={`Reboot ${device.name || device.mac}`}
                on:click={() => onReboot(device)}
                disabled={rowBusy[device.mac]?.refresh ||
                  rowBusy[device.mac]?.remove ||
                  rowBusy[device.mac]?.reboot}
                >{#if rowBusy[device.mac]?.reboot}<span
                    class="spinner-border spinner-border-sm"
                    aria-hidden="true"
                  ></span>{:else}<span aria-hidden="true">⏻</span>{/if}</button
              >
              <button
                class="btn btn-sm btn-outline-danger row-action-btn"
                title="Delete this device"
                aria-label={`Delete ${device.name || device.mac}`}
                on:click={() => onRemove(device)}
                disabled={rowBusy[device.mac]?.refresh ||
                  rowBusy[device.mac]?.remove ||
                  rowBusy[device.mac]?.reboot}><span aria-hidden="true">🗑</span></button
              >
            </div>
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

{#if showEmpty}
  <div class="alert alert-secondary mt-3 mb-0">
    No devices loaded yet. Start a scan or refresh this page.
  </div>
{/if}

<style>
  :global(tr.device-stale td) {
    opacity: 0.62;
  }

  :global(tr.device-stale .badge) {
    opacity: 1;
  }

  :global(tr.device-stale .row-action-btn) {
    opacity: 1;
  }
</style>
