import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { after, before, describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import pg from 'pg';

// Runs the real command as its own process and asserts the index the database ended up with,
// which is the part a reader cannot check by eye.
// Requires TEST_DATABASE_ADMIN_URL with the migration chain applied; skips without one.

const ADMIN_URL = process.env.TEST_DATABASE_ADMIN_URL;
const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SCRIPT = 'scripts/db-vector-index.mjs';
const INDEX_NAME = 'idx_product_embedding';

// A unit vector of the width product.embedding holds.
const EMBEDDING = `[${Array.from({ length: 1536 }, (_, i) => (i === 0 ? 1 : 0)).join(',')}]`;

function run(...args) {
  return spawnSync(process.execPath, [SCRIPT, ...args], {
    cwd: ROOT,
    encoding: 'utf8',
    env: { ...process.env, DATABASE_ADMIN_URL: ADMIN_URL },
  });
}

describe(
  'db-vector-index.mjs',
  { skip: ADMIN_URL ? false : 'TEST_DATABASE_ADMIN_URL is not set' },
  () => {
    let client;
    let accountID;
    let preexistingDefinition = null;

    before(async () => {
      client = new pg.Client({ connectionString: ADMIN_URL });
      await client.connect();
      // The whole definition, not just whether it exists: the tests below rebuild the index with
      // their own lists, and putting back an index with the wrong partition count is no better
      // than leaving none.
      const existing = await client.query('SELECT indexdef FROM pg_indexes WHERE indexname = $1', [
        INDEX_NAME,
      ]);
      preexistingDefinition = existing.rows[0]?.indexdef ?? null;

      const account = await client.query(
        `INSERT INTO account (name) VALUES ('Vector index coverage') RETURNING id`,
      );
      accountID = account.rows[0].id;
      await client.query(
        `INSERT INTO product (account_id, canonical_name, embedding) VALUES ($1, $2, $3)`,
        [accountID, 'Cemento Portland 50kg', EMBEDDING],
      );
    });

    after(async () => {
      await client.query(`DROP INDEX IF EXISTS ${INDEX_NAME}`);
      // An index that was already there is somebody else's, so it goes back as it was.
      if (preexistingDefinition) await client.query(preexistingDefinition);
      if (accountID) {
        await client.query('DELETE FROM product WHERE account_id = $1', [accountID]);
        await client.query('DELETE FROM account WHERE id = $1', [accountID]);
      }
      await client.end();
    });

    async function indexDefinition() {
      const { rows } = await client.query('SELECT indexdef FROM pg_indexes WHERE indexname = $1', [
        INDEX_NAME,
      ]);
      return rows[0]?.indexdef ?? null;
    }

    it('creates the index over the embedded catalog', async () => {
      const result = run();
      assert.equal(result.status, 0, result.stderr);
      assert.match(result.stdout, /Indexed \d+ embedded product\(s\) with lists = \d+\./);

      const definition = await indexDefinition();
      assert.ok(definition, 'the index should exist');
      assert.match(definition, /USING ivfflat \(embedding vector_cosine_ops\)/);
    });

    it('rebuilds it with an operator-set number of partitions', async () => {
      const result = run('--lists', '3');
      assert.equal(result.status, 0, result.stderr);
      assert.match(result.stdout, /lists = 3\./);

      assert.match(await indexDefinition(), /lists='3'/);
    });

    it('refuses a partition count that is not a positive integer', async () => {
      const result = run('--lists', 'plenty');
      assert.equal(result.status, 1);
      assert.match(result.stderr, /--lists takes a positive integer/);
    });
  },
);
