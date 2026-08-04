import { fileURLToPath } from 'node:url';
import react from '@vitejs/plugin-react';

function stub(name) {
  return fileURLToPath(new URL(`./stubs/${name}.js`, import.meta.url));
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
      'server-only': stub('server-only'),
    },
  },
  test: {
    environment: 'jsdom',
    include: ['**/*.test.{ts,tsx}'],
    exclude: ['**/node_modules/**', '**/dist/**', '**/.next/**'],
    setupFiles: [stub('setup')],
    // Reported, never enforced — a threshold set before the suites are real is met by writing
    // tests that assert nothing. Same rule the API follows.
    coverage: {
      reporter: ['text-summary', 'json-summary'],
      include: ['app/**', 'components/**', 'config/**', 'lib/**', 'src/**'],
    },
  },
};
