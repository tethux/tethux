import js from '@eslint/js';
import globals from 'globals';
import svelte from 'eslint-plugin-svelte';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  { ignores: ['build/', '.svelte-kit/'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...svelte.configs['flat/recommended'],
  {
    languageOptions: { globals: { ...globals.browser, ...globals.node } }
  },
  {
    files: ['**/*.svelte'],
    languageOptions: { parserOptions: { parser: tseslint.parser } }
  }
);
