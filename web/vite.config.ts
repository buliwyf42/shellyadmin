import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { readFileSync } from 'node:fs'

const pkg = JSON.parse(readFileSync(new URL('./package.json', import.meta.url), 'utf8')) as { version?: string }
const appVersion = pkg.version ?? '0.0.0-dev'

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
      '/login': 'http://127.0.0.1:8080',
      '/logout': 'http://127.0.0.1:8080',
      '/health': 'http://127.0.0.1:8080',
    },
  },
  build: {
    outDir: 'dist',
  },
})
