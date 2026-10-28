import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import path from 'node:path';
import test from 'node:test';
import pg from 'pg';

const ADMIN_URL = process.env.TEST_DATABASE_ADMIN_URL;
const API_DIR = path.resolve('apps/api');

test(
  'client contact migrations retain legacy data, dedupe shared contacts and refuse duplicate fiscal profiles',
  { skip: ADMIN_URL ? false : 'TEST_DATABASE_ADMIN_URL is required' },
  async () => {
    const name = 'contacts_migration_' + randomUUID().replaceAll('-', '');
    const role = 'contacts_guard_' + randomUUID().replaceAll('-', '');
    const admin = new pg.Client({ connectionString: ADMIN_URL });
    await admin.connect();
    const url = new URL(ADMIN_URL);
    url.pathname = '/' + name;
    let client;
    try {
      await admin.query(`CREATE DATABASE ${name}`);
      const goose = (command, target) =>
        spawnSync(
          'go',
          [
            'tool',
            'goose',
            '-dir',
            'migrations',
            'postgres',
            url.toString(),
            command,
            String(target),
          ],
          { cwd: API_DIR, encoding: 'utf8' },
        );
      const migrate = (target) => goose('up-to', target);
      const stderr = (result) => result.stderr?.replaceAll(url.toString(), '[database]');
      let result = migrate(59);
      assert.equal(result.status, 0, stderr(result));
      client = new pg.Client({ connectionString: url.toString() });
      await client.connect();
      const account = randomUUID(),
        otherAccount = randomUUID(),
        branch = randomUUID(),
        requestBranch = randomUUID(),
        otherOwner = randomUUID(),
        nationalAfterCode = randomUUID(),
        foreignPrefix = randomUUID(),
        first = randomUUID(),
        duplicate = randomUUID(),
        phoneOwner = randomUUID(),
        phoneCopy = randomUUID(),
        emailOlder = randomUUID(),
        emailNewer = randomUUID(),
        fiscalOlder = randomUUID(),
        fiscalNewer = randomUUID(),
        unparseable = randomUUID();
      await client.query('INSERT INTO account(id,name) VALUES($1,$2)', [
        account,
        'Contact migration fixture',
      ]);
      await client.query('INSERT INTO account(id,name) VALUES($1,$2)', [
        otherAccount,
        'Other supplier',
      ]);
      await client.query(
        "INSERT INTO branch(id,account_id,name,created_at) VALUES($1,$2,'Central','2026-01-01'),($3,$2,'Norte','2026-01-02'),(gen_random_uuid(),$4,'Otra','2026-01-01')",
        [branch, account, requestBranch, otherAccount],
      );
      const insertClient = (id, created, fields) =>
        client.query(
          'INSERT INTO client(id,account_id,created_at,name,tax_id,phone,email) VALUES($1,$2,$3,$4,$5,$6,$7)',
          [
            id,
            fields.account ?? account,
            created,
            fields.name ?? null,
            fields.taxId ?? null,
            fields.phone ?? null,
            fields.email ?? null,
          ],
        );
      await insertClient(first, '2026-01-01', {
        name: 'Customer',
        taxId: '20123456786',
        phone: '+54 9 (11) 5555-0101',
        email: ' ORIGINAL@EXAMPLE.TEST ',
      });
      await insertClient(duplicate, '2026-01-02', { taxId: '20123456786' });
      // The CUIT profile keeps a shared phone even though the other copy is newer.
      await insertClient(phoneOwner, '2026-01-03', {
        taxId: '27111111110',
        phone: '011 5555-0202',
      });
      await insertClient(phoneCopy, '2026-02-03', { name: 'Walk-in', phone: '1155550202' });
      await insertClient(emailOlder, '2026-01-04', { email: 'shared@example.test' });
      await insertClient(emailNewer, '2026-02-04', { email: ' Shared@Example.TEST' });
      await insertClient(fiscalOlder, '2026-01-05', {
        taxId: '30222222220',
        phone: '0221 555-0303',
      });
      await insertClient(fiscalNewer, '2026-02-05', {
        taxId: '30333333330',
        phone: '549 221 555 0303',
      });
      await insertClient(unparseable, '2026-01-06', { phone: '15 5555 0404' });
      await insertClient(nationalAfterCode, '2026-01-07', { phone: '54 11 5555-0606' });
      await insertClient(foreignPrefix, '2026-01-08', { phone: '0011 5555 0505' });
      const malformedEmails = [
        'trailing@example.com.',
        'x\u0001y@example.com',
        'a..b@example.com',
        '<angle@example.com>',
        'semi@example.com;',
        'mailto:a@example.com',
      ];
      const malformed = malformedEmails.map(() => randomUUID());
      for (const [index, email] of malformedEmails.entries()) {
        await insertClient(malformed[index], '2026-01-09', { email });
      }
      // Another supplier's profile with the same phone is independent: no ranking crosses accounts.
      await insertClient(otherOwner, '2026-03-01', { account: otherAccount, phone: '1155550101' });
      // The shared copy's review lands in the branch that served the profile, not the oldest one.
      const channel = randomUUID();
      await client.query(
        "INSERT INTO channel(id,account_id,branch_id,type,identifier) VALUES($1,$2,$3,'EMAIL','norte@example.test')",
        [channel, account, requestBranch],
      );
      await client.query(
        'INSERT INTO rfq(account_id,branch_id,channel_id,client_id) VALUES($1,$2,$3,$4)',
        [account, requestBranch, channel, phoneCopy],
      );

      result = migrate(60);
      assert.notEqual(result.status, 0, 'Duplicate profiles must stop migration');
      assert.equal(
        (
          await client.query('SELECT count(*)::int AS count FROM client WHERE account_id=$1', [
            account,
          ])
        ).rows[0].count,
        17,
      );
      await client.query('DELETE FROM client WHERE id=$1', [duplicate]);
      result = migrate(63);
      assert.equal(result.status, 0, stderr(result));

      const contacts = await client.query(
        'SELECT client_id,kind,value,original_value,is_primary,source,verified_at IS NOT NULL AS verified FROM client_contact WHERE account_id=$1',
        [account],
      );
      const contact = (id, kind) =>
        contacts.rows.find((row) => row.client_id === id && row.kind === kind);
      assert.deepEqual(contact(first, 'EMAIL'), {
        client_id: first,
        kind: 'EMAIL',
        value: 'original@example.test',
        original_value: ' ORIGINAL@EXAMPLE.TEST ',
        is_primary: true,
        source: 'LEGACY',
        verified: true,
      });
      assert.deepEqual(contact(first, 'WHATSAPP'), {
        client_id: first,
        kind: 'WHATSAPP',
        value: '+5491155550101',
        original_value: '+54 9 (11) 5555-0101',
        is_primary: true,
        source: 'LEGACY',
        verified: true,
      });
      const verification = (id, kind) => {
        const row = contact(id, kind);
        return { value: row.value, verified: row.verified };
      };
      assert.deepEqual(verification(phoneOwner, 'WHATSAPP'), {
        value: '+5491155550202',
        verified: true,
      });
      assert.deepEqual(verification(phoneCopy, 'WHATSAPP'), {
        value: '+5491155550202',
        verified: false,
      });
      assert.deepEqual(verification(emailNewer, 'EMAIL'), {
        value: 'shared@example.test',
        verified: true,
      });
      assert.deepEqual(verification(emailOlder, 'EMAIL'), {
        value: 'shared@example.test',
        verified: false,
      });
      assert.deepEqual(verification(fiscalNewer, 'WHATSAPP'), {
        value: '+5492215550303',
        verified: true,
      });
      assert.deepEqual(verification(fiscalOlder, 'WHATSAPP'), {
        value: '+5492215550303',
        verified: false,
      });
      assert.deepEqual(verification(unparseable, 'WHATSAPP'), {
        value: '1555550404',
        verified: false,
      });
      assert.deepEqual(verification(nationalAfterCode, 'WHATSAPP'), {
        value: '+5491155550606',
        verified: true,
      });
      assert.deepEqual(verification(foreignPrefix, 'WHATSAPP'), {
        value: '001155550505',
        verified: false,
      });
      for (const id of malformed) assert.equal(contact(id, 'EMAIL').verified, false);
      const other = await client.query(
        "SELECT c.value,c.verified_at IS NOT NULL AS verified,(SELECT count(*)::int FROM client_contact_observation o WHERE o.account_id=$1) AS observations FROM client_contact c WHERE c.client_id=$2 AND c.kind='WHATSAPP'",
        [otherAccount, otherOwner],
      );
      assert.deepEqual(other.rows, [{ value: '+5491155550101', verified: true, observations: 0 }]);

      const observationQuery = () =>
        client.query(
          'SELECT branch_id,message_id,source_client_id,candidate_client_id,tax_id,kind,value,declared_name,original_text,status FROM client_contact_observation WHERE account_id=$1 ORDER BY kind::text,value',
          [account],
        );
      const observations = await observationQuery();
      // tax_id stays empty so the losing CUIT profile's customer is not told a review is pending.
      const review = (source, candidate, reviewBranch, kind, value, declaredName, original) => ({
        branch_id: reviewBranch,
        message_id: null,
        source_client_id: source,
        candidate_client_id: candidate,
        tax_id: null,
        kind,
        value,
        declared_name: declaredName,
        original_text: original,
        status: 'PENDING',
      });
      assert.deepEqual(observations.rows, [
        review(
          emailOlder,
          emailNewer,
          branch,
          'EMAIL',
          'shared@example.test',
          null,
          'shared@example.test',
        ),
        review(
          phoneCopy,
          phoneOwner,
          requestBranch,
          'WHATSAPP',
          '+5491155550202',
          'Walk-in',
          '1155550202',
        ),
        review(
          fiscalOlder,
          fiscalNewer,
          branch,
          'WHATSAPP',
          '+5492215550303',
          null,
          '0221 555-0303',
        ),
      ]);

      result = goose('down-to', 61);
      assert.equal(result.status, 0, stderr(result));
      result = migrate(63);
      assert.equal(result.status, 0, stderr(result));
      assert.deepEqual((await observationQuery()).rows, observations.rows);

      const legacy = await client.query('SELECT phone,email FROM client WHERE id=$1', [first]);
      assert.deepEqual(legacy.rows[0], {
        phone: '+54 9 (11) 5555-0101',
        email: ' ORIGINAL@EXAMPLE.TEST ',
      });
      await assert.rejects(
        client.query("INSERT INTO client(account_id,tax_id) VALUES($1,'20123456786')", [account]),
        (error) => error.code === '23505',
      );

      // A role under row level security must stop a data migration instead of updating nothing.
      await client.query(`CREATE ROLE ${role} NOLOGIN`);
      await client.query('CREATE TABLE guard_probe (id INTEGER)');
      await client.query(`ALTER TABLE guard_probe OWNER TO ${role}`);
      await client.query('ALTER TABLE guard_probe ENABLE ROW LEVEL SECURITY');
      await client.query(`SET ROLE ${role}`);
      await client.query("SELECT assert_migration_sees_all_rows('guard_probe')");
      await assert.rejects(
        client.query("SELECT assert_migration_sees_all_rows('guard_probe','client')"),
        /Row level security filters client/,
      );
      await client.query('RESET ROLE');
      await client.query('ALTER TABLE guard_probe FORCE ROW LEVEL SECURITY');
      await client.query(`SET ROLE ${role}`);
      await assert.rejects(
        client.query("SELECT assert_migration_sees_all_rows('guard_probe')"),
        /Row level security filters guard_probe/,
      );
      await client.query('RESET ROLE');
      await client.query('DROP TABLE guard_probe');
    } finally {
      if (client) await client.end();
      await admin.query(`DROP DATABASE IF EXISTS ${name}`);
      await admin.query(`DROP ROLE IF EXISTS ${role}`);
      await admin.end();
    }
  },
);
