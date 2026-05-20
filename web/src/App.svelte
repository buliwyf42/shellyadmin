<script lang="ts">
  import { onMount } from 'svelte';
  import { currentPath, navigate, uiScale } from './lib/stores';
  import { api } from './lib/api';
  import Navbar from './components/Navbar.svelte';
  import LoginPage from './pages/Login.svelte';
  import SetupPage from './pages/Setup.svelte';
  import DevicesPage from './pages/Devices.svelte';
  import DeviceDetailPage from './pages/DeviceDetail.svelte';
  import ScanPage from './pages/Scan.svelte';
  import FirmwarePage from './pages/Firmware.svelte';
  import ProvisionPage from './pages/Provision.svelte';
  import GroupsPage from './pages/Groups.svelte';
  import CompliancePage from './pages/Compliance.svelte';
  import SettingsPage from './pages/Settings.svelte';
  import LogsPage from './pages/Logs.svelte';
  import AboutPage from './pages/About.svelte';
  import DocsPage from './pages/Docs.svelte';

  const routes = {
    '/login': LoginPage,
    '/setup': SetupPage,
    '/': DevicesPage,
    '/scan': ScanPage,
    '/firmware': FirmwarePage,
    '/provision': ProvisionPage,
    '/groups': GroupsPage,
    '/compliance': CompliancePage,
    '/logs': LogsPage,
    '/settings': SettingsPage,
    '/about': AboutPage,
    '/docs': DocsPage,
  } as const;

  function resolvePage(path: string) {
    if (path.startsWith('/devices/')) return DeviceDetailPage;
    return routes[path as keyof typeof routes] ?? DevicesPage;
  }

  // First-run gate: ask the server whether an operator account exists. When it
  // doesn't, force the setup screen; when it does, bounce away from /setup.
  onMount(async () => {
    try {
      const { configured } = await api.setupStatus();
      if (!configured && window.location.pathname !== '/setup') {
        navigate('/setup');
      } else if (configured && window.location.pathname === '/setup') {
        navigate('/login');
      }
    } catch {
      // Status probe failed (server unreachable) — leave routing as-is; the
      // normal 401-redirect path still guards authenticated views.
    }
  });

  $: Page = resolvePage($currentPath);
  $: showShell = $currentPath !== '/login' && $currentPath !== '/setup';
  $: if (typeof document !== 'undefined') {
    document.documentElement.dataset.uiScale = $uiScale;
  }
</script>

{#if showShell}
  <Navbar />
{/if}

<main class={showShell ? 'container-fluid py-4' : ''}>
  <svelte:component this={Page} />
</main>
