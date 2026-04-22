<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import { currentPath, uiScale } from '../lib/stores';
  import type { VersionInfo } from '../lib/types';
  import { APP_VERSION } from '../lib/version';

  const links = [
    { path: '/', label: 'Devices', icon: 'grid' },
    { path: '/scan', label: 'Scan', icon: 'search' },
    { path: '/firmware', label: 'Firmware', icon: 'chip' },
    { path: '/provision', label: 'Provision', icon: 'upload' },
    { path: '/groups', label: 'Auth Groups', icon: 'layers' },
    { path: '/compliance', label: 'Compliance', icon: 'shield' },
    { path: '/logs', label: 'Logs', icon: 'list' },
    { path: '/settings', label: 'Settings', icon: 'gear' },
    { path: '/docs', label: 'API Docs', icon: 'book' },
    { path: '/about', label: 'About', icon: 'info' },
  ] as const;

  let runtimeVersion: VersionInfo = { backend_version: '', commit: '' };

  onMount(async () => {
    try {
      runtimeVersion = await api.getVersion();
    } catch {
      runtimeVersion = { backend_version: '', commit: '' };
    }
  });

  $: navVersion = runtimeVersion.backend_version || APP_VERSION;

  function iconPath(name: (typeof links)[number]['icon'] | 'logout' | 'resize'): string {
    switch (name) {
      case 'grid':
        return 'M3 3h4v4H3V3zm7 0h4v4h-4V3zm7 0h4v4h-4V3zM3 10h4v4H3v-4zm7 0h4v4h-4v-4zm7 0h4v4h-4v-4zM3 17h4v4H3v-4zm7 0h4v4h-4v-4zm7 0h4v4h-4v-4z';
      case 'search':
        return 'M10.5 3a7.5 7.5 0 015.978 12.032l3.745 3.745-1.06 1.06-3.745-3.745A7.5 7.5 0 1110.5 3zm0 1.5a6 6 0 100 12 6 6 0 000-12z';
      case 'chip':
        return 'M8 4h8v2H8V4zm-3 3h14v10H5V7zm2 2v6h10V9H7zm1 10h8v2H8v-2zM4 8h2v8H4V8zm14 0h2v8h-2V8z';
      case 'upload':
        return 'M12 3l4 4h-3v6h-2V7H8l4-4zm-7 10h2v5h10v-5h2v7H5v-7z';
      case 'layers':
        return 'M12 3l9 5-9 5-9-5 9-5zm0 7.2l8.1-4.5m-16.2 0L12 10.2m9 3.8l-9 5-9-5';
      case 'gear':
        return 'M11 2h2l.4 2.1a7.7 7.7 0 011.7.7l1.8-1.2 1.4 1.4-1.2 1.8c.3.5.6 1.1.7 1.7L21 11v2l-2.1.4c-.2.6-.4 1.2-.7 1.7l1.2 1.8-1.4 1.4-1.8-1.2c-.5.3-1.1.6-1.7.7L13 22h-2l-.4-2.1c-.6-.2-1.2-.4-1.7-.7l-1.8 1.2-1.4-1.4 1.2-1.8a7.7 7.7 0 01-.7-1.7L3 13v-2l2.1-.4c.2-.6.4-1.2.7-1.7L4.6 7.1 6 5.7l1.8 1.2c.5-.3 1.1-.6 1.7-.7L11 2zm1 6a4 4 0 100 8 4 4 0 000-8z';
      case 'shield':
        return 'M12 2l8 3v5c0 5.3-3.3 9.2-8 11-4.7-1.8-8-5.7-8-11V5l8-3zm0 2.1L6 6.3V10c0 4.3 2.5 7.5 6 9 3.5-1.5 6-4.7 6-9V6.3l-6-2.2zm-1 4.4h2v4h-2v-4zm0 5.5h2v2h-2v-2z';
      case 'list':
        return 'M4 5h2v2H4V5zm4 0h12v2H8V5zM4 11h2v2H4v-2zm4 0h12v2H8v-2zM4 17h2v2H4v-2zm4 0h12v2H8v-2z';
      case 'book':
        return 'M5 4.5A2.5 2.5 0 017.5 2H20v17H7.5A2.5 2.5 0 005 21.5V4.5zm2.5-1A1 1 0 006.5 4.5v14A3.5 3.5 0 017.5 18H18.5V3.5h-11z';
      case 'info':
        return 'M12 3a9 9 0 110 18 9 9 0 010-18zm0 1.5a7.5 7.5 0 100 15 7.5 7.5 0 000-15zm-1 5h2v7h-2v-7zm0-3h2v2h-2v-2z';
      case 'logout':
        return 'M10 4h-5v16h5v2H3V2h7v2zm4.6 3.4L13.2 8.8 15.4 11H8v2h7.4l-2.2 2.2 1.4 1.4L19.2 12l-4.6-4.6z';
      case 'resize':
        return 'M4 4h6v2H6v4H4V4zm10 0h6v6h-2V6h-4V4zM4 14h2v4h4v2H4v-6zm14 0h2v6h-6v-2h4v-4z';
    }
  }

  function isActive(path: string): boolean {
    if (path === '/' && $currentPath.startsWith('/devices/')) {
      return true;
    }
    return $currentPath === path;
  }

  async function logout() {
    await api.logout();
    window.location.href = '/login';
  }
</script>

<nav class="navbar topbar border-bottom border-secondary-subtle bg-black">
  <div class="container-fluid">
    <a href="/" class="brand text-decoration-none text-light fw-bold">
      <img src="/logo-mark.svg" alt="ShellyAdmin logo" class="brand-logo" />
      <span class="brand-name">ShellyAdmin</span>
      <span class="brand-version">v{navVersion}</span>
    </a>
    <div class="topnav-main">
      {#each links as link}
        <a href={link.path} class={`topnav-link ${isActive(link.path) ? 'is-active' : ''}`}>
          <span class="topnav-icon" aria-hidden="true">
            <svg viewBox="0 0 24 24" role="img">
              <path d={iconPath(link.icon)} />
            </svg>
          </span>
          <span>{link.label}</span>
        </a>
      {/each}
    </div>
    <div class="topnav-side">
      <label class="topnav-scale" for="nav-ui-scale" title="UI size">
        <span class="topnav-scale-label">Size</span>
        <select id="nav-ui-scale" class="topnav-scale-select" bind:value={$uiScale}>
          <option value="compact">S</option>
          <option value="default">M</option>
          <option value="large">L</option>
          <option value="xlarge">XL</option>
          <option value="xxlarge">XXL</option>
        </select>
      </label>
      <button type="button" class="topnav-link logout" on:click={logout}>
        <span class="topnav-icon" aria-hidden="true">
          <svg viewBox="0 0 24 24" role="img">
            <path d={iconPath('logout')} />
          </svg>
        </span>
        <span>Logout</span>
      </button>
    </div>
  </div>
</nav>
