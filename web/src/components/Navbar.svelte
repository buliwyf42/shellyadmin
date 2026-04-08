<script lang="ts">
  import { currentPath, navigate } from '../lib/stores'

  const links = [
    ['/', 'Devices'],
    ['/scan', 'Scan'],
    ['/firmware', 'Firmware'],
    ['/compliance', 'Compliance'],
    ['/provision', 'Provision'],
    ['/settings', 'Settings'],
    ['/logs', 'Logs'],
  ] as const

  async function logout() {
    await fetch('/logout', { method: 'POST', credentials: 'same-origin' })
    navigate('/login')
  }
</script>

<nav class="navbar navbar-expand-lg border-bottom border-secondary-subtle bg-black">
  <div class="container-fluid">
    <a href="/" class="navbar-brand btn btn-link text-decoration-none text-light fw-bold" on:click|preventDefault={() => navigate('/')}>
      ShellyAdmin
    </a>
    <div class="navbar-nav flex-wrap">
      {#each links as [path, label]}
        <a
          href={path}
          class={`btn btn-sm me-2 mb-2 ${$currentPath === path ? 'btn-warning text-dark' : 'btn-outline-light'}`}
          on:click|preventDefault={() => navigate(path)}
        >
          {label}
        </a>
      {/each}
      <button type="button" class="btn btn-sm btn-outline-danger mb-2" on:click={logout}>Logout</button>
    </div>
  </div>
</nav>
