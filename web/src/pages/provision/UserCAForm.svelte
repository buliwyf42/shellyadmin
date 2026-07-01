<script lang="ts">
  import { APIError, api } from '../../lib/api';
  import type { Device, UploadUserCAResult } from '../../lib/types';
  import SectionCard from '../../components/SectionCard.svelte';
  import Select from '../../components/Select.svelte';

  type CertKind = 'user_ca' | 'tls_client_cert' | 'tls_client_key';

  export let devices: Device[] = [];
  export let selected: Set<string>;

  let pem = '';
  let fileName = '';
  let kind: CertKind = 'user_ca';
  let open = false;
  let uploading = false;
  let error = '';
  let results: UploadUserCAResult[] = [];

  const kindOptions: Array<{ value: CertKind; label: string; description: string }> = [
    {
      value: 'user_ca',
      label: 'User CA (user_ca.pem)',
      description: 'Shelly.PutUserCA — used by ssl_ca = "user_ca.pem"',
    },
    {
      value: 'tls_client_cert',
      label: 'TLS Client Cert',
      description: 'Shelly.PutTLSClientCert — mTLS client certificate for MQTT/WS brokers',
    },
    {
      value: 'tls_client_key',
      label: 'TLS Client Key',
      description: 'Shelly.PutTLSClientKey — mTLS client private key for MQTT/WS brokers',
    },
  ];

  $: selectedDevices = devices.filter((d) => selected.has(d.mac));
  $: canUpload = !uploading && pem.trim().length > 0 && selectedDevices.length > 0;
  $: pemLooksValid = pem.trim() === '' || pem.includes('-----BEGIN');
  $: kindLabel = kindOptions.find((o) => o.value === kind)?.label ?? kind;
  $: sectionTag =
    kind === 'user_ca' ? 'user ca' : kind === 'tls_client_cert' ? 'tls cert' : 'tls key';

  async function onFileSelected(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    if (file.size > 64 * 1024) {
      error = `File too large (${file.size} bytes); max is 64KB.`;
      return;
    }
    try {
      pem = await file.text();
      fileName = file.name;
      error = '';
    } catch (err) {
      error = `Failed to read file: ${(err as Error).message}`;
    }
  }

  function clearPEM() {
    pem = '';
    fileName = '';
    error = '';
    results = [];
  }

  async function upload() {
    error = '';
    results = [];
    if (!canUpload) return;
    uploading = true;
    try {
      const ips = selectedDevices.map((d) => d.ip);
      results = await api.uploadUserCA(ips, pem, kind);
    } catch (err) {
      if (err instanceof APIError) {
        error = err.message;
      } else {
        error = (err as Error).message;
      }
    } finally {
      uploading = false;
    }
  }

  function statusClass(status: string): string {
    switch (status) {
      case 'ok':
        return 'bg-success';
      case 'failed':
        return 'bg-danger';
      case 'skipped':
        return 'bg-warning text-dark';
      default:
        return 'bg-secondary';
    }
  }
</script>

<SectionCard tag={sectionTag} title="Upload Certificate (PEM)" bind:open>
  <p class="text-secondary mb-2 text-hint-lg">
    Pushes a PEM certificate to the device via chunked <code>Shelly.Put*</code> RPCs. Required
    before MQTT/WS configs referencing <code>user_ca.pem</code> or the mTLS client cert/key take effect.
  </p>

  <div class="mb-2 mw-22r">
    <label class="form-label" for="user-ca-kind">Certificate kind</label>
    <Select bind:value={kind} options={kindOptions} ariaLabel="Certificate kind" />
  </div>

  <div class="d-flex gap-2 flex-wrap mb-2 align-items-center">
    <label class="btn btn-sm btn-outline-light mb-0">
      Choose PEM file…
      <input
        type="file"
        accept=".pem,.crt,.cer,.key,.txt,application/x-pem-file"
        on:change={onFileSelected}
        hidden
      />
    </label>
    {#if fileName}
      <span class="text-secondary text-hint-md">{fileName}</span>
    {/if}
    <button
      type="button"
      class="btn btn-sm btn-outline-secondary"
      on:click={clearPEM}
      disabled={!pem && results.length === 0}>Clear</button
    >
  </div>

  <label class="form-label" for="user-ca-pem">PEM content</label>
  <textarea
    id="user-ca-pem"
    class="form-control font-monospace"
    rows="6"
    placeholder="-----BEGIN CERTIFICATE-----&#10;...&#10;-----END CERTIFICATE-----"
    bind:value={pem}></textarea>
  {#if !pemLooksValid}
    <div class="text-warning mt-1 text-hint-md">
      Warning: content does not contain a PEM header.
    </div>
  {/if}

  <div class="d-flex gap-2 align-items-center mt-2 flex-wrap">
    <button
      type="button"
      class="btn btn-sm btn-warning text-dark"
      on:click={upload}
      disabled={!canUpload}
    >
      {uploading
        ? 'Uploading…'
        : `Upload ${kindLabel} to ${selectedDevices.length} device${selectedDevices.length === 1 ? '' : 's'}`}
    </button>
    {#if selectedDevices.length === 0}
      <span class="text-secondary text-hint-md"
        >Select at least one device in the list on the left.</span
      >
    {/if}
  </div>

  {#if error}
    <div class="alert alert-danger mt-2 py-2 mb-0" role="alert">{error}</div>
  {/if}

  {#if results.length > 0}
    <div class="table-responsive mt-2" role="status" aria-live="polite">
      <table class="table table-dark table-striped table-sm mb-0">
        <thead>
          <tr><th>IP</th><th>Status</th><th>Chunks</th><th>Bytes</th><th>Detail</th></tr>
        </thead>
        <tbody>
          {#each results as result (result.ip)}
            <tr>
              <td>{result.ip}</td>
              <td><span class={`badge ${statusClass(result.status)}`}>{result.status}</span></td>
              <td>{result.chunks}</td>
              <td>{result.bytes_sent}</td>
              <td>{result.detail}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</SectionCard>
