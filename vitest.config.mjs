import { defineConfig } from 'vitest/config';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  resolve: {
    alias: {
      '@agentflow/live-spec-core': path.resolve(__dirname, 'packages/live-spec-core/src'),
      '@agentflow/dsh-interactive-spec': path.resolve(__dirname, 'packages/dsh-interactive-spec/src'),
    },
  },
  test: {
    include: ['packages/**/*.{test,spec}.ts', 'tests/**/*.{test,spec}.ts'],
    exclude: ['**/node_modules/**', '**/dist/**', 'scripts/**'],
  },
});
