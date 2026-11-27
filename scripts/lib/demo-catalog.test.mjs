import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { test } from 'node:test';

import { assertLocalDemoEnvironment, demoOptions } from './demo-catalog.mjs';

test('demo refuses production, remote URLs and unknown flags before writes', () => {
  for (const env of [{ ENV: 'production' }, { NODE_ENV: 'production' }]) {
    assert.throws(() => assertLocalDemoEnvironment('postgres://localhost/coti', env));
  }
  assert.throws(() => assertLocalDemoEnvironment('postgres://example.com/coti', {}));
  assert.doesNotThrow(() => assertLocalDemoEnvironment('postgres://127.0.0.1/coti', {}));
  assert.throws(() => demoOptions(['--apply', '--dry-run']));
  assert.throws(() => demoOptions(['--account=foreign']));
  assert.equal(demoOptions([]).apply, false);
  const run = spawnSync(process.execPath, ['scripts/demo-catalog.mjs', '--apply'], {
    encoding: 'utf8',
    env: {
      ...process.env,
      ENV: 'production',
      DATABASE_ADMIN_URL: 'postgres://localhost/coti',
      DATABASE_URL: 'postgres://localhost/coti',
    },
  });
  assert.notEqual(run.status, 0);
  assert.match(run.stderr, /refused in production/);
  assert.doesNotMatch(run.stdout, /Applying dev seed/);
});
