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
    // Set VITE_ALLOWED_HOSTS (comma-separated) to reach the dev server via a
    // non-localhost hostname, e.g. VITE_ALLOWED_HOSTS=devhost.lan
    // Ternary (not ??) so an empty string falls through to ['localhost']
    // rather than producing [''] which either blocks all requests or allows all.
    allowedHosts: process.env.VITE_ALLOWED_HOSTS
      ? process.env.VITE_ALLOWED_HOSTS.split(',')
      : ['localhost'],
    proxy: {
      '/api': 'http://127.0.0.1:8080',
      '/health': 'http://127.0.0.1:8080',
    },
  },
  build: {
    outDir: 'dist',
    // Vite 8 made oxc the default minifier and unbundled esbuild. v0.2.7
    // adopted oxc to drop the esbuild devDep (added in v0.2.0 just to keep
    // byte-stable output across the v6→v8 rolldown jump). oxc is also
    // rolldown's native transformer, so the build pipeline is single-tooled.
    minify: 'oxc',
    target: 'es2020',
    cssMinify: true,
    reportCompressedSize: true,
    // Keep a single app chunk for now; we'll revisit if we add route-level
    // code splitting. Raise the warning so CI output stays quiet — the hard
    // budget is enforced by scripts/check-bundle-size.mjs.
    chunkSizeWarningLimit: 500,
  },
});
