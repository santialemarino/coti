import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import { after, describe, it } from 'node:test';
import pg from 'pg';

import { appRoleCredentials, setAppRolePassword } from './app-role.mjs';

// Requires TEST_DATABASE_ADMIN_URL; skips without one. The role it creates is its own, never
// coti_app, whose password cannot be read back and so could not be put back.
const ADMIN_URL = process.env.TEST_DATABASE_ADMIN_URL;

describe('appRoleCredentials', () => {
  it('reads the role and its password out of a connection url', () => {
    const { role, password } = appRoleCredentials(
      'postgres://coti_app:s3cret@localhost:5433/coti?sslmode=disable',
    );
    assert.equal(role, 'coti_app');
    assert.equal(password, 's3cret');
  });

  it('decodes a password that had to be escaped', () => {
    const { password } = appRoleCredentials('postgres://coti_app:p%40ss%3Aword@localhost/coti');
    assert.equal(password, 'p@ss:word');
  });

  it('refuses a url with no password, rather than setting an empty one', () => {
    assert.throws(
      () => appRoleCredentials('postgres://coti_app@localhost/coti'),
      /must carry the app role and its password/,
    );
  });
});

describe(
  'setAppRolePassword',
  { skip: ADMIN_URL ? false : 'TEST_DATABASE_ADMIN_URL is not set' },
  () => {
    const role = `coti_app_test_${crypto.randomBytes(6).toString('hex')}`;
    const password = crypto.randomBytes(12).toString('hex');
    let admin;

    after(async () => {
      if (!admin) return;
      await admin.query(`DROP ROLE IF EXISTS ${admin.escapeIdentifier(role)}`);
      await admin.end();
    });

    it('turns a login role that cannot authenticate into one that can', async () => {
      admin = new pg.Client({ connectionString: ADMIN_URL });
      await admin.connect();
      await admin.query(`CREATE ROLE ${admin.escapeIdentifier(role)} LOGIN`);

      await assert.rejects(connectAs(role, password), /password authentication failed/);

      await setAppRolePassword(admin, { role, password });
      await connectAs(role, password);
    });

    async function connectAs(user, secret) {
      const url = new URL(ADMIN_URL);
      url.username = user;
      url.password = secret;
      const client = new pg.Client({ connectionString: url.toString() });
      await client.connect();
      await client.end();
    }
  },
);
