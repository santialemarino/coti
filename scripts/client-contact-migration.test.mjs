import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import path from 'node:path';
import test from 'node:test';
import pg from 'pg';

const ADMIN_URL = process.env.TEST_DATABASE_ADMIN_URL;
const API_DIR = path.resolve('apps/api');

test(
  'client contact migrations retain legacy data and refuse duplicate fiscal profiles',
  { skip: ADMIN_URL ? false : 'TEST_DATABASE_ADMIN_URL is required' },
  async () => {
    const name = 'contacts_migration_' + randomUUID().replaceAll('-', '');
    const admin = new pg.Client({ connectionString: ADMIN_URL });
    await admin.connect();
    const url = new URL(ADMIN_URL);
    url.pathname = '/' + name;
    let client;
    try {
      await admin.query(`CREATE DATABASE ${name}`);
      const migrate = (target) =>
        spawnSync(
          'go',
          [
            'tool',
            'goose',
            '-dir',
            'migrations',
            'postgres',
            url.toString(),
            'up-to',
            String(target),
          ],
          { cwd: API_DIR, encoding: 'utf8' },
        );
      let result = migrate(55);
      assert.equal(result.status, 0, result.stderr?.replaceAll(url.toString(), '[database]'));
      client = new pg.Client({ connectionString: url.toString() });
      await client.connect();
      const account = randomUUID(),
        first = randomUUID(),
        duplicate = randomUUID();
      await client.query('INSERT INTO account(id,name) VALUES($1,$2)', [
        account,
        'Contact migration fixture',
      ]);
      await client.query(
        "INSERT INTO client(id,account_id,name,tax_id,phone,email) VALUES($1,$2,'Customer','20123456786','+54 9 (11) 5555-0101',' ORIGINAL@EXAMPLE.TEST ')",
        [first, account],
      );
      await client.query("INSERT INTO client(id,account_id,tax_id) VALUES($1,$2,'20123456786')", [
        duplicate,
        account,
      ]);
      result = migrate(56);
      assert.notEqual(result.status, 0, 'Duplicate profiles must stop migration');
      assert.equal(
        (
          await client.query('SELECT count(*)::int AS count FROM client WHERE account_id=$1', [
            account,
          ])
        ).rows[0].count,
        2,
      );
      await client.query('DELETE FROM client WHERE id=$1', [duplicate]);
      result = migrate(57);
      assert.equal(result.status, 0, result.stderr?.replaceAll(url.toString(), '[database]'));
      const contacts = await client.query(
        'SELECT kind,value,original_value,is_primary,source FROM client_contact WHERE account_id=$1 ORDER BY kind::text',
        [account],
      );
      assert.deepEqual(contacts.rows, [
        {
          kind: 'EMAIL',
          value: 'original@example.test',
          original_value: ' ORIGINAL@EXAMPLE.TEST ',
          is_primary: true,
          source: 'LEGACY',
        },
        {
          kind: 'WHATSAPP',
          value: '+5491155550101',
          original_value: '+54 9 (11) 5555-0101',
          is_primary: true,
          source: 'LEGACY',
        },
      ]);
      const legacy = await client.query('SELECT phone,email FROM client WHERE id=$1', [first]);
      assert.deepEqual(legacy.rows[0], {
        phone: '+54 9 (11) 5555-0101',
        email: ' ORIGINAL@EXAMPLE.TEST ',
      });
      await assert.rejects(
        client.query("INSERT INTO client(account_id,tax_id) VALUES($1,'20123456786')", [account]),
        (error) => error.code === '23505',
      );
    } finally {
      if (client) await client.end();
      await admin.query(`DROP DATABASE IF EXISTS ${name}`);
      await admin.end();
    }
  },
);
