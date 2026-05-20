// Pure navbar logic, extracted from Navbar.svelte so it can be unit-tested
// without rendering the component (mirrors sortHeader.ts ↔ SortHeader.svelte).

/**
 * Normalize the version string for the navbar badge. The badge template
 * prefixes "v", so strip a leading "v" the backend may already carry —
 * release images embed the git tag (e.g. "v0.3.5") — to avoid "vv0.3.5".
 */
export function normalizeNavVersion(raw: string): string {
  return raw.replace(/^v/, '');
}

/**
 * Whether a nav link should render as active for the current route. The
 * Devices link ("/") also owns the device-detail routes ("/devices/:id");
 * every other link matches its path exactly.
 */
export function isNavLinkActive(path: string, currentPath: string): boolean {
  if (path === '/' && currentPath.startsWith('/devices/')) {
    return true;
  }
  return currentPath === path;
}
