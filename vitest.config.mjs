import { defineConfig } from 'vitest/config';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  resolve: {
    alias: {
      '@agentflow/live-spec-core': path.resolve(__dirname, 'packages/live-spec-core/src'),
      '@agentflow/dsh-interactive-spec': path.resolve(__dirname, 'packages/dsh-interactive-spec/src'),
      'react': path.resolve(__dirname, 'node_modules/.pnpm/react@18.3.1/node_modules/react'),
      'react-dom': path.resolve(__dirname, 'node_modules/.pnpm/react-dom@18.3.1_react@18.3.1/node_modules/react-dom'),
    },
  },
  test: {
    include: ['packages/**/*.{test,spec}.ts', 'tests/**/*.{test,spec}.ts'],
    exclude: ['**/node_modules/**', '**/dist/**', 'scripts/**'],
  },
});
