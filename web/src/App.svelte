<script lang="ts">
  import { currentPath } from './lib/stores'
  import Navbar from './components/Navbar.svelte'
  import LoginPage from './pages/Login.svelte'
  import DevicesPage from './pages/Devices.svelte'
  import ScanPage from './pages/Scan.svelte'
  import FirmwarePage from './pages/Firmware.svelte'
  import ProvisionPage from './pages/Provision.svelte'
  import CompliancePage from './pages/Compliance.svelte'
  import SettingsPage from './pages/Settings.svelte'
  import LogsPage from './pages/Logs.svelte'
  import AboutPage from './pages/About.svelte'

  const routes = {
    '/login': LoginPage,
    '/': DevicesPage,
    '/scan': ScanPage,
    '/firmware': FirmwarePage,
    '/provision': ProvisionPage,
    '/compliance': CompliancePage,
    '/settings': SettingsPage,
    '/logs': LogsPage,
    '/about': AboutPage,
  } as const

  $: Page = routes[$currentPath as keyof typeof routes] ?? DevicesPage
  $: showShell = $currentPath !== '/login'
</script>

{#if showShell}
  <Navbar />
{/if}

<main class={showShell ? 'container-fluid py-4' : ''}>
  <svelte:component this={Page} />
</main>
