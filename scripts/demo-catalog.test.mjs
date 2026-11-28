import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { test } from 'node:test';
import pg from 'pg';

test(
  'the real demo command is idempotent and preserves another account',
  {
    skip:
      process.env.TEST_DATABASE_ADMIN_URL && process.env.TEST_DATABASE_URL
        ? false
        : 'Both integration database roles are required',
  },
  async () => {
    const client = new pg.Client({ connectionString: process.env.TEST_DATABASE_ADMIN_URL });
    await client.connect();
    const account = randomUUID();
    try {
      await client.query('INSERT INTO account(id,name) VALUES($1,$2)', [
        account,
        'Demo isolation test',
      ]);
      const run = () =>
        spawnSync(process.execPath, ['scripts/demo-catalog.mjs', '--apply', '--skip-embeddings'], {
          encoding: 'utf8',
          env: {
            ...process.env,
            ENV: 'development',
            NODE_ENV: 'test',
            DATABASE_ADMIN_URL: process.env.TEST_DATABASE_ADMIN_URL,
            DATABASE_URL: process.env.TEST_DATABASE_URL,
          },
        });
      const first = run();
      assert.equal(first.status, 0, first.stderr);
      const before = await client.query(
        'SELECT id,code,canonical_name,embedding_updated_at FROM product WHERE account_id=$1 ORDER BY id',
        ['a0000000-0000-4000-8000-000000000001'],
      );
      const second = run();
      assert.equal(second.status, 0, second.stderr);
      const after = await client.query(
        'SELECT id,code,canonical_name,embedding_updated_at FROM product WHERE account_id=$1 ORDER BY id',
        ['a0000000-0000-4000-8000-000000000001'],
      );
      assert.deepEqual(after.rows, before.rows);
      assert.equal(
        (await client.query('SELECT name FROM account WHERE id=$1', [account])).rows[0].name,
        'Demo isolation test',
      );
    } finally {
      await client.query('DELETE FROM account WHERE id=$1', [account]);
      await client.end();
    }
  },
);
