import { defineConfig } from 'vitest/config';

export default defineConfig({
  define: {
    __APP_VERSION__: JSON.stringify('0.0.0-test'),
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.ts'],
    globals: false,
    coverage: {
      provider: 'v8',
      // Count every .ts source, imported by a test or not — otherwise a new
      // untested module is invisible to the gate. Svelte components are out
      // of scope: this config has no svelte plugin, so component logic that
      // wants coverage gets extracted into .ts modules (the navbar.ts /
      // state.ts pattern) where it is testable in the first place.
      include: ['src/**/*.ts'],
      exclude: ['src/**/*.test.ts', 'src/**/*.d.ts'],
      // Floor set ~6 points under the measured baseline (36%/38% at
      // introduction) — same regression-guard spirit as the Go 45% gate.
      // Bump when the measured number durably exceeds it.
      thresholds: {
        statements: 30,
        lines: 30,
      },
    },
  },
});
