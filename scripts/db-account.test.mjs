import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { after, before, describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import pg from 'pg';

// Runs the real command as its own process and asserts what changed in the database, not what
// it printed: the printing is the part a reader checks by eye, the writes are not.
// Requires TEST_DATABASE_ADMIN_URL with the migration chain applied; skips without one.

const ADMIN_URL = process.env.TEST_DATABASE_ADMIN_URL;
const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SCRIPT = 'scripts/db-account.mjs';
const UNKNOWN_ID = '00000000-0000-4000-8000-0000000000ff';

function run(...args) {
  return spawnSync(process.execPath, [SCRIPT, ...args], {
    cwd: ROOT,
    encoding: 'utf8',
    env: { ...process.env, DATABASE_ADMIN_URL: ADMIN_URL },
  });
}

describe(
  'db-account.mjs',
  { skip: ADMIN_URL ? false : 'TEST_DATABASE_ADMIN_URL is not set' },
  () => {
    let client;
    let accountID;
    const userIDs = [];

    before(async () => {
      client = new pg.Client({ connectionString: ADMIN_URL });
      await client.connect();
      const account = await client.query(
        `INSERT INTO account (name) VALUES ('Script coverage') RETURNING id`,
      );
      accountID = account.rows[0].id;
      for (const name of ['Uno', 'Dos']) {
        const user = await client.query(
          `INSERT INTO app_user (account_id, name, email, password_hash, role, session_epoch)
         VALUES ($1, $2, $3, 'x', 'ADMIN', 1) RETURNING id`,
          [accountID, name, `${accountID}-${name}@test.local`],
        );
        userIDs.push(user.rows[0].id);
      }
    });

    after(async () => {
      if (accountID) {
        await client.query('DELETE FROM app_user WHERE account_id = $1', [accountID]);
        await client.query('DELETE FROM account WHERE id = $1', [accountID]);
      }
      await client.end();
    });

    async function state() {
      const { rows } = await client.query(
        `SELECT a.is_active, (SELECT sum(session_epoch) FROM app_user WHERE account_id = a.id) AS epochs
       FROM account a WHERE a.id = $1`,
        [accountID],
      );
      return { isActive: rows[0].is_active, epochs: Number(rows[0].epochs) };
    }

    it('deactivates the account and bumps every user of it', async () => {
      const before = await state();
      assert.equal(before.isActive, true, 'the fixture should start active');

      const result = run('deactivate', '--account', accountID);
      assert.equal(result.status, 0, result.stderr);
      assert.match(result.stdout, /Deactivated "Script coverage"/);

      const after = await state();
      assert.equal(after.isActive, false);
      // Two users, each +1 — the count, not just "something moved".
      assert.equal(after.epochs, before.epochs + userIDs.length);
    });

    it('reactivates without touching the session epoch', async () => {
      const before = await state();

      const result = run('activate', '--account', accountID);
      assert.equal(result.status, 0, result.stderr);
      assert.match(result.stdout, /Activated "Script coverage"/);

      const after = await state();
      assert.equal(after.isActive, true);
      assert.equal(after.epochs, before.epochs, 'activate must not bump the epoch');
    });

    it('refuses an unknown id without writing anything', async () => {
      const before = await state();

      const result = run('deactivate', '--account', UNKNOWN_ID);
      assert.equal(result.status, 1);
      assert.match(result.stderr, /No account with id/);

      assert.deepEqual(await state(), before);
    });

    it('refuses a malformed id before it opens a connection', async () => {
      const result = run('deactivate', '--account', 'not-a-uuid');
      assert.equal(result.status, 1);
      assert.match(result.stderr, /--account takes an account uuid/);
    });

    it('refuses an unknown action', async () => {
      const before = await state();

      const result = run('disable', '--account', accountID);
      assert.equal(result.status, 1);
      assert.match(result.stderr, /Usage:/);

      assert.deepEqual(await state(), before);
    });
  },
);
