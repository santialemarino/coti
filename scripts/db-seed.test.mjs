import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';

// Runs the real command as its own process. What matters here is WHICH database it reaches: the
// script used to shell into a container named in the source, so the address it connects to could
// not be changed and POSTGRES_USER / POSTGRES_DB were ignored outright.
// Requires TEST_DATABASE_ADMIN_URL with the migration chain applied; skips without one.

const ADMIN_URL = process.env.TEST_DATABASE_ADMIN_URL;
const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SCRIPT = 'scripts/db-seed.mjs';

// A port nothing is listening on. Reaching a container regardless would make this succeed.
const UNUSED_ADDRESS = '127.0.0.1:5999';

function run(adminUrl) {
  return spawnSync(process.execPath, [SCRIPT], {
    cwd: ROOT,
    encoding: 'utf8',
    env: { ...process.env, DATABASE_ADMIN_URL: adminUrl },
  });
}

describe('db-seed.mjs', { skip: ADMIN_URL ? false : 'TEST_DATABASE_ADMIN_URL is not set' }, () => {
  it('seeds the database its URL names, and says so when it cannot reach it', () => {
    const unreachable = run(`postgres://coti:coti@${UNUSED_ADDRESS}/coti?sslmode=disable`);

    assert.notEqual(unreachable.status, 0, 'a database it cannot reach must fail the command');
    assert.match(
      unreachable.stderr,
      new RegExp(UNUSED_ADDRESS.replace('.', '\\.')),
      'the failure must name the address it tried, which is what makes a wrong URL diagnosable',
    );
  });

  // The seed is applied on every db:init, so re-running it has to be a no-op rather than a
  // duplicate-key failure.
  it('applies cleanly twice over', () => {
    const first = run(ADMIN_URL);
    assert.equal(first.status, 0, `first run failed: ${first.stderr}`);

    const second = run(ADMIN_URL);
    assert.equal(second.status, 0, `second run failed: ${second.stderr}`);
  });
});
