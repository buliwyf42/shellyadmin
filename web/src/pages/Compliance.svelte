<script lang="ts">
  import { onMount } from 'svelte';
  import { api, toErrorDetails, toErrorMessage } from '../lib/api';
  import { devices } from '../lib/stores';
  import type { AppSettings } from '../lib/types';
  import ErrorNotice from '../components/ErrorNotice.svelte';
  import ComplianceRulesForm from './compliance/ComplianceRulesForm.svelte';
  import DeviceMatrix from './compliance/DeviceMatrix.svelte';

  let settings: AppSettings = {
    subnets: [],
    scan_timeout: 2,
    refresh_timeout: 5,
    scan_concurrency: 64,
    enable_mdns: false,
    advanced_mode_enabled: false,
    compliance: { custom_rules: [] },
  };
  let loading = false;
  let error = '';
  let errorDetails = '';

  function captureError(err: unknown) {
    error = toErrorMessage(err);
    errorDetails = toErrorDetails(err);
  }

  async function load() {
    loading = true;
    error = '';
    errorDetails = '';
    try {
      const [settingsResult, devicesResult] = await Promise.allSettled([
        api.getSettings(),
        api.getDevices(),
      ]);

      if (settingsResult.status === 'fulfilled') {
        settings = settingsResult.value;
      } else {
        captureError(settingsResult.reason);
      }

      if (devicesResult.status === 'fulfilled') {
        $devices = devicesResult.value;
      } else {
        if (!error) {
          captureError(devicesResult.reason);
        }
        $devices = [];
      }
    } finally {
      loading = false;
    }
  }

  // ComplianceRulesForm.save calls this after applyTogglesToSettings has
  // flushed the toggle state back onto `settings`. We persist + reload;
  // throw on failure so the child knows not to flash "Saved".
  async function persistAndReload() {
    try {
      await api.saveSettings(settings);
      await load();
    } catch (err) {
      captureError(err);
      throw err;
    }
  }

  onMount(() => void load());
</script>

<ErrorNotice summary={error} details={errorDetails} />

<div class="row g-3">
  <div class="col-lg-6">
    <ComplianceRulesForm bind:settings {loading} onSave={persistAndReload} />
  </div>

  <div class="col-lg-6">
    <DeviceMatrix {loading} />
  </div>
</div>

<!-- Styles for the extracted children live in compliance/CustomRulesList.svelte
     (.sa-custom-rule); the rest of the dark/Bootstrap surface is global. -->
