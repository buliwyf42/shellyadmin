<script lang="ts">
  import { api } from '../lib/api'
  import { currentPath } from '../lib/stores'
  import { APP_VERSION } from '../lib/version'

  const links = [
    ['/', 'Devices'],
    ['/scan', 'Scan'],
    ['/firmware', 'Firmware'],
    ['/compliance', 'Compliance'],
    ['/provision', 'Provision'],
    ['/settings', 'Settings'],
    ['/logs', 'Logs'],
    ['/about', 'About'],
  ] as const

  async function logout() {
    await api.logout()
    window.location.href = '/login'
  }
</script>

<nav class="navbar navbar-expand-lg border-bottom border-secondary-subtle bg-black">
  <div class="container-fluid">
    <a href="/" class="navbar-brand btn btn-link text-decoration-none text-light fw-bold">
      ShellyAdmin
      <span class="badge bg-secondary ms-2">v{APP_VERSION}</span>
    </a>
    <div class="navbar-nav flex-wrap">
      {#each links as [path, label]}
        <a
          href={path}
          class={`btn btn-sm me-2 mb-2 ${$currentPath === path ? 'btn-warning text-dark' : 'btn-outline-light'}`}
        >
          {label}
        </a>
      {/each}
      <button type="button" class="btn btn-sm btn-outline-danger mb-2" on:click={logout}>Logout</button>
    </div>
  </div>
</nav>
