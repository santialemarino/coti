import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import pg from 'pg';

const ADMIN_URL = process.env.TEST_DATABASE_ADMIN_URL;
const API_DIR = path.resolve('apps/api');
const MIGRATIONS_DIR = path.join(API_DIR, 'migrations');
const MIGRATION_VERSIONS = fs
  .readdirSync(MIGRATIONS_DIR)
  .filter((filename) => /^\d+_.*\.sql$/.test(filename))
  .map((filename) => Number.parseInt(filename, 10))
  .sort((left, right) => left - right);
const HISTORY_SQL = fs
  .readFileSync('docs/technical/database.md', 'utf8')
  .split('### Recommendation branch databases')[1]
  ?.match(/```sql\r?\n([\s\S]*?)```/)?.[1];

test(
  'recommendation migrations preserve data from dev and reconcile branch-only history',
  { skip: ADMIN_URL ? false : 'TEST_DATABASE_ADMIN_URL is required' },
  async () => {
    assert.ok(HISTORY_SQL, 'the documented history reconciliation must be available');
    const admin = new pg.Client({ connectionString: ADMIN_URL });
    await admin.connect();
    const scratch = fs.mkdtempSync(path.join(os.tmpdir(), 'coti-recommendation-migrations-'));
    try {
      for (const filename of fs.readdirSync(MIGRATIONS_DIR)) {
        const version = Number.parseInt(filename, 10);
        if (version <= 59 || (version >= 74 && version <= 76)) {
          const destination =
            version >= 74 ? String(version - 14).padStart(5, '0') + filename.slice(5) : filename;
          fs.copyFileSync(path.join(MIGRATIONS_DIR, filename), path.join(scratch, destination));
        }
      }
      for (const legacy of [false, true]) {
        const name = 'recommendation_migration_' + randomUUID().replaceAll('-', '');
        const url = new URL(ADMIN_URL);
        url.pathname = '/' + name;
        let client;
        await admin.query(`CREATE DATABASE ${name}`);
        try {
          const migrate = (directory, args, allowMissing = false) => {
            const result = spawnSync(
              'go',
              [
                'tool',
                'goose',
                ...(allowMissing ? ['-allow-missing'] : []),
                '-dir',
                directory,
                'postgres',
                url.toString(),
                ...args,
              ],
              { cwd: API_DIR, encoding: 'utf8' },
            );
            assert.equal(
              result.status,
              0,
              (result.stderr ?? result.error?.message ?? '').replaceAll(
                url.toString(),
                '[database]',
              ),
            );
          };
          migrate(legacy ? scratch : MIGRATIONS_DIR, legacy ? ['up'] : ['up-to', '62']);
          client = new pg.Client({ connectionString: url.toString() });
          await client.connect();
          const accountID = randomUUID();
          const baseID = randomUUID();
          const alternativeID = randomUUID();
          await client.query('INSERT INTO account(id,name) VALUES($1,$2)', [
            accountID,
            'Recommendation migration fixture',
          ]);
          await client.query(
            'INSERT INTO product(id,account_id,canonical_name) VALUES($1,$3,$4),($2,$3,$5)',
            [baseID, alternativeID, accountID, 'Base product', 'Economy product'],
          );
          await client.query(
            `INSERT INTO product_alternative(account_id,base_product_id,alternative_product_id,type)
             VALUES($1,$2,$3,'ECONOMY')`,
            [accountID, baseID, alternativeID],
          );
          if (legacy) {
            await client.query(HISTORY_SQL);
            const history = await client.query(
              'SELECT version_id::int FROM goose_db_version WHERE version_id > 59 ORDER BY version_id',
            );
            assert.deepEqual(
              history.rows.map((row) => row.version_id),
              [74, 75, 76],
            );
          } else {
            await assert.rejects(client.query(HISTORY_SQL), /does not match the branch-only/);
            await client.query('ROLLBACK');
            const history = await client.query(
              'SELECT max(version_id)::int AS version FROM goose_db_version',
            );
            assert.equal(history.rows[0].version, 62);
            migrate(MIGRATIONS_DIR, ['up-to', '73']);
          }
          migrate(MIGRATIONS_DIR, ['up'], legacy);
          const history = await client.query(
            'SELECT version_id::int FROM goose_db_version WHERE is_applied ORDER BY version_id',
          );
          assert.deepEqual(
            history.rows.map((row) => row.version_id),
            [0, ...MIGRATION_VERSIONS],
          );
          const data = await client.query(
            `SELECT account.name, product.canonical_name, alternative.type
             FROM account
             JOIN product ON product.account_id = account.id AND product.id = $2
             JOIN product_alternative alternative ON alternative.account_id = account.id
               AND alternative.base_product_id = product.id AND alternative.alternative_product_id = $3
             WHERE account.id = $1`,
            [accountID, baseID, alternativeID],
          );
          assert.deepEqual(data.rows, [
            {
              name: 'Recommendation migration fixture',
              canonical_name: 'Base product',
              type: 'ECONOMY',
            },
          ]);
          await assert.rejects(client.query(HISTORY_SQL), /does not match the branch-only/);
          await client.query('ROLLBACK');
          migrate(MIGRATIONS_DIR, ['up']);
        } finally {
          if (client) await client.end();
          await admin.query(`DROP DATABASE ${name} WITH (FORCE)`);
        }
      }
    } finally {
      await admin.end();
      const target = fs.realpathSync(scratch);
      assert.equal(path.dirname(target), fs.realpathSync(os.tmpdir()));
      assert.ok(path.basename(target).startsWith('coti-recommendation-migrations-'));
      fs.rmSync(target, { recursive: true });
    }
  },
);
