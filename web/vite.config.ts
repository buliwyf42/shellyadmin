import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { readFileSync } from 'node:fs';

const pkg = JSON.parse(readFileSync(new URL('./package.json', import.meta.url), 'utf8')) as {
  version?: string;
};
const appVersion = pkg.version ?? '0.0.0-dev';

export default defineConfig({
  plugins: [svelte()],
  define: {
    __APP_VERSION__: JSON.stringify(appVersion),
  },
  server: {
    host: true,
    port: 5173,
    strictPort: true,
    allowedHosts: ['devhost.home.lan'],
    proxy: {
      '/api': 'http://127.0.0.1:8080',
      '/health': 'http://127.0.0.1:8080',
    },
  },
  build: {
    outDir: 'dist',
    // Explicit minification with esbuild (Vite default) — document the choice
    // so future maintainers don't accidentally lose it.
    minify: 'esbuild',
    target: 'es2020',
    cssMinify: true,
    reportCompressedSize: true,
    // Keep a single app chunk for now; we'll revisit if we add route-level
    // code splitting. Raise the warning so CI output stays quiet — the hard
    // budget is enforced by scripts/check-bundle-size.mjs.
    chunkSizeWarningLimit: 500,
  },
});
