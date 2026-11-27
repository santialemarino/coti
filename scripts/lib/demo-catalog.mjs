import { spawnSync } from 'node:child_process';

export const DEMO_ACCOUNT_ID = 'a0000000-0000-4000-8000-000000000001';

export function assertLocalDemoEnvironment(url, environment = process.env) {
  if ([environment.ENV, environment.NODE_ENV].includes('production')) {
    throw new Error('Demo operations are refused in production.');
  }
  const parsed = new URL(url);
  if (
    !['postgres:', 'postgresql:'].includes(parsed.protocol) ||
    !['localhost', '127.0.0.1', '[::1]'].includes(parsed.hostname)
  ) {
    throw new Error('Demo operations require an explicitly local PostgreSQL database.');
  }
}

export function demoOptions(args) {
  const allowed = new Set(['--apply', '--dry-run', '--benchmark', '--skip-embeddings']);
  if (
    args.some((arg) => !allowed.has(arg)) ||
    (args.includes('--apply') && args.includes('--dry-run'))
  ) {
    throw new Error(
      'Usage: demo-catalog.mjs [--dry-run | --apply] [--skip-embeddings] [--benchmark]',
    );
  }
  return {
    apply: args.includes('--apply'),
    benchmark: args.includes('--benchmark'),
    embeddings: !args.includes('--skip-embeddings'),
  };
}

export function runDemoCommand(command, args, options = {}) {
  const result = spawnSync(command, args, { stdio: 'inherit', ...options });
  if (result.error || result.status !== 0)
    throw new Error('Demo step failed; completed embedding batches remain resumable.');
}
