import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import path from 'node:path';
import test from 'node:test';
import pg from 'pg';

const ADMIN_URL = process.env.TEST_DATABASE_ADMIN_URL;
const API_DIR = path.resolve('apps/api');

test(
  'the outbox status time keeps an existing prompt dated when it was queued',
  { skip: ADMIN_URL ? false : 'TEST_DATABASE_ADMIN_URL is required' },
  async () => {
    const name = 'outbox_migration_' + randomUUID().replaceAll('-', '');
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
      const stderr = (result) => result.stderr?.replaceAll(url.toString(), '[database]');
      let result = migrate(64);
      assert.equal(result.status, 0, stderr(result));
      client = new pg.Client({ connectionString: url.toString() });
      await client.connect();
      const account = randomUUID(),
        branch = randomUUID(),
        channel = randomUUID();
      await client.query("INSERT INTO account(id,name) VALUES($1,'Outbox fixture')", [account]);
      await client.query("INSERT INTO branch(id,account_id,name) VALUES($1,$2,'Central')", [
        branch,
        account,
      ]);
      await client.query(
        "INSERT INTO channel(id,account_id,branch_id,type,identifier) VALUES($1,$2,$3,'EMAIL','ventas@example.test')",
        [channel, account, branch],
      );
      await client.query(
        "INSERT INTO client_contact_outbox(account_id,branch_id,channel_id,kind,value,purpose,status,created_at) VALUES($1,$2,$3,'EMAIL','old@example.test','REQUEST_NAME','SENT','2026-01-01T00:00:00Z')",
        [account, branch, channel],
      );

      result = migrate(65);
      assert.equal(result.status, 0, stderr(result));
      const row = await client.query(
        "SELECT status_changed_at=created_at AS dated FROM client_contact_outbox WHERE value='old@example.test'",
      );
      assert.deepEqual(row.rows, [{ dated: true }]);
    } finally {
      if (client) await client.end();
      await admin.query(`DROP DATABASE IF EXISTS ${name}`);
      await admin.end();
    }
  },
);
