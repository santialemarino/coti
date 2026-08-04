import { fileURLToPath } from 'node:url';
import react from '@vitejs/plugin-react';

function localPath(name) {
  return fileURLToPath(new URL(name, import.meta.url));
}

export const vitestConfig = {
  plugins: [react()],
  resolve: {
    // Each package imports through the aliases its own tsconfig declares, so the runner reads
    // that map rather than keeping a second copy of it to drift against.
    tsconfigPaths: true,
    alias: {
      /*
       * Next resolves the `server-only` marker in its own bundler and ships no package for it,
       * so every module carrying that import is unresolvable under a plain runner without this.
       */
      'server-only': localPath('./stubs/server-only.js'),
    },
  },
  test: {
    environment: 'jsdom',
    include: ['**/*.test.{ts,tsx}'],
    exclude: ['**/node_modules/**', '**/dist/**', '**/.next/**'],
    setupFiles: [localPath('./setup.js')],
    // Reported, never enforced — a threshold set before the suites are real is met by writing
    // tests that assert nothing. Same rule the API follows.
    coverage: {
      reporter: ['text-summary', 'json-summary'],
      include: ['app/**', 'components/**', 'config/**', 'lib/**', 'src/**'],
    },
  },
};
