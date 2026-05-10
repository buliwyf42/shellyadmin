import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import prettier from 'eslint-config-prettier';
import sveltePlugin from 'eslint-plugin-svelte';
import svelteParser from 'svelte-eslint-parser';

export default [
  {
    ignores: [
      'dist/**',
      'node_modules/**',
      'public/**',
      '*.config.js',
      '*.config.ts',
      'scripts/**',
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...sveltePlugin.configs['flat/recommended'],
  {
    languageOptions: {
      globals: {
        window: 'readonly',
        document: 'readonly',
        console: 'readonly',
        fetch: 'readonly',
        localStorage: 'readonly',
        sessionStorage: 'readonly',
        setTimeout: 'readonly',
        clearTimeout: 'readonly',
        setInterval: 'readonly',
        clearInterval: 'readonly',
        history: 'readonly',
        location: 'readonly',
        navigator: 'readonly',
        URL: 'readonly',
        URLSearchParams: 'readonly',
        FormData: 'readonly',
        FileReader: 'readonly',
        Blob: 'readonly',
        Event: 'readonly',
        CustomEvent: 'readonly',
        HTMLElement: 'readonly',
        HTMLInputElement: 'readonly',
        HTMLTextAreaElement: 'readonly',
        HTMLSelectElement: 'readonly',
        HTMLFormElement: 'readonly',
        HTMLDivElement: 'readonly',
        HTMLButtonElement: 'readonly',
        HTMLAnchorElement: 'readonly',
        HTMLSpanElement: 'readonly',
        Node: 'readonly',
        KeyboardEvent: 'readonly',
        MouseEvent: 'readonly',
        FocusEvent: 'readonly',
        DragEvent: 'readonly',
        WheelEvent: 'readonly',
        confirm: 'readonly',
        alert: 'readonly',
        prompt: 'readonly',
        requestAnimationFrame: 'readonly',
        cancelAnimationFrame: 'readonly',
      },
    },
    rules: {
      '@typescript-eslint/no-unused-vars': [
        'warn',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
      ],
      '@typescript-eslint/no-explicit-any': 'off',
      'no-empty': ['error', { allowEmptyCatch: true }],
      // v0.2.0-prep: rules below newly-default in ESLint 10 / eslint-plugin-svelte 3.
      // Disabled here to keep the dep-bump pass mechanical; each surfaces real
      // migration work tracked as separate follow-ups:
      //   - svelte/require-each-key (~16 sites): add stable keys to {#each} blocks
      //   - svelte/prefer-svelte-reactivity (4 sites): migrate `new Set` → SvelteSet
      //     (changes reactivity pattern from immutable-reassignment to mutation)
      //   - svelte/no-useless-mustaches (UserCAForm.svelte:139): trivial cleanup
      //   - no-useless-assignment (Provision.svelte:291,298): control-flow analysis
      //     across conditional branches in `autoSelectedCredentialRef` writes
      'svelte/require-each-key': 'off',
      'svelte/prefer-svelte-reactivity': 'off',
    },
  },
  {
    files: ['**/*.svelte'],
    languageOptions: {
      parser: svelteParser,
      parserOptions: {
        parser: tseslint.parser,
      },
    },
    rules: {
      // svelte-eslint-parser + @typescript-eslint/no-unused-vars has a known
      // compatibility bug that crashes on .svelte files. Svelte's compiler
      // already warns on unused props; rely on that instead.
      '@typescript-eslint/no-unused-vars': 'off',
      // TypeScript already checks for undefined identifiers (including
      // generic type parameters); the core rule misfires on TS syntax in
      // Svelte files.
      'no-undef': 'off',
    },
  },
  prettier,
];
