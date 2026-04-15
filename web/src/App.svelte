<script lang="ts">
  import { currentPath, uiScale } from './lib/stores'
  import Navbar from './components/Navbar.svelte'
  import LoginPage from './pages/Login.svelte'
  import DevicesPage from './pages/Devices.svelte'
  import DeviceDetailPage from './pages/DeviceDetail.svelte'
  import ScanPage from './pages/Scan.svelte'
  import FirmwarePage from './pages/Firmware.svelte'
  import ProvisionPage from './pages/Provision.svelte'
  import GroupsPage from './pages/Groups.svelte'
  import CompliancePage from './pages/Compliance.svelte'
  import SettingsPage from './pages/Settings.svelte'
  import LogsPage from './pages/Logs.svelte'
  import AboutPage from './pages/About.svelte'
  import DocsPage from './pages/Docs.svelte'

  const routes = {
    '/login': LoginPage,
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
  } as const

  function resolvePage(path: string) {
    if (path.startsWith('/devices/')) return DeviceDetailPage
    return routes[path as keyof typeof routes] ?? DevicesPage
  }

  $: Page = resolvePage($currentPath)
  $: showShell = $currentPath !== '/login'
  $: if (typeof document !== 'undefined') {
    document.documentElement.dataset.uiScale = $uiScale
  }
</script>

{#if showShell}
  <Navbar />
{/if}

<main class={showShell ? 'container-fluid py-4' : ''}>
  <svelte:component this={Page} />
</main>
