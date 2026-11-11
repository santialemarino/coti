import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import path from 'node:path';
import test from 'node:test';
import pg from 'pg';

const ADMIN_URL = process.env.TEST_DATABASE_ADMIN_URL;
const API_DIR = path.resolve('apps/api');

test(
  'migration 00077 keeps only proved contacts verified, writes Argentine phones as +549 and sends clashes to review',
  { skip: ADMIN_URL ? false : 'TEST_DATABASE_ADMIN_URL is required' },
  async () => {
    const name = 'contacts_canonical_' + randomUUID().replaceAll('-', '');
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
      const stderr = (result) => result.stderr?.replaceAll(url.toString(), '[database]');
      let result = goose('up-to', 76);
      assert.equal(result.status, 0, stderr(result));
      client = new pg.Client({ connectionString: url.toString() });
      await client.connect();

      const account = randomUUID(),
        branch = randomUUID(),
        typedOld = randomUUID(),
        channelNew = randomUUID(),
        twin = randomUUID(),
        registered = randomUUID(),
        identity = randomUUID();
      await client.query("INSERT INTO account(id,name) VALUES($1,'Canonical phones')", [account]);
      await client.query("INSERT INTO branch(id,account_id,name) VALUES($1,$2,'Central')", [
        branch,
        account,
      ]);
      const insertClient = (id, fields) =>
        client.query(
          'INSERT INTO client(id,account_id,name,tax_id,phone,email) VALUES($1,$2,$3,$4,$5,$6)',
          [
            id,
            account,
            fields.name,
            fields.taxId ?? null,
            fields.phone ?? null,
            fields.email ?? null,
          ],
        );
      const addContact = (clientId, value, source, primary) =>
        client.query(
          'INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source,verified_at) VALUES($1,$2,$3,$4,$5,$6,now())',
          [account, clientId, value.includes('@') ? 'EMAIL' : 'WHATSAPP', value, primary, source],
        );
      // A seller typed the number without the 9; the default made it verified.
      await insertClient(typedOld, {
        name: 'Obra Belgrano',
        phone: '+54 11 5555-0202',
        email: 'obra@example.test',
      });
      // The same person wrote on WhatsApp, which reports the number with the 9.
      await insertClient(channelNew, { name: '+5491155550202' });
      await addContact(channelNew, '+5491155550202', 'CHANNEL', true);
      // One profile holding both forms keeps one row.
      await insertClient(twin, { name: 'Twin', phone: '+541155550303' });
      await addContact(twin, '+5491155550303', 'CHANNEL', false);
      // A contact the customer proved stays verified.
      await insertClient(registered, { name: 'Registered', taxId: '20123456786' });
      await client.query(
        "INSERT INTO customer_identity(id,account_id,tax_id,password_hash) VALUES($1,$2,'20123456786','unused')",
        [identity, account],
      );
      const proved = (
        await client.query(
          "INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source,verified_at) VALUES($1,$2,'EMAIL','proved@example.test',TRUE,'CUSTOMER',now()) RETURNING id",
          [account, registered],
        )
      ).rows[0].id;
      await client.query(
        'INSERT INTO customer_contact_ownership(account_id,customer_id,contact_id,includes_profile_history) VALUES($1,$2,$3,TRUE)',
        [account, identity, proved],
      );
      const channel = randomUUID();
      await client.query(
        "INSERT INTO channel(id,account_id,branch_id,type) VALUES($1,$2,$3,'WHATSAPP')",
        [channel, account, branch],
      );
      await client.query(
        "INSERT INTO client_contact_outbox(account_id,branch_id,channel_id,kind,value,purpose) VALUES($1,$2,$3,'WHATSAPP','+541155550404','REQUEST_NAME')",
        [account, branch, channel],
      );
      // A seller typed this one, but a WhatsApp message from it was already routed to the profile.
      const routed = randomUUID();
      await insertClient(routed, { name: 'Routed', phone: '+54 11 5555-0505' });
      const rfq = (
        await client.query(
          "INSERT INTO rfq(account_id,branch_id,channel_id,raw_text,client_id) VALUES($1,$2,$3,'Pedido',$4) RETURNING id",
          [account, branch, channel, routed],
        )
      ).rows[0].id;
      const message = (
        await client.query(
          "INSERT INTO inbound_channel_message(account_id,branch_id,channel_id,rfq_id,external_message_id,sender_id,body,payload,provider_received_at) VALUES($1,$2,$3,$4,'wamid.1','5491155550505','Pedido','{}',now()) RETURNING id",
          [account, branch, channel, rfq],
        )
      ).rows[0].id;
      await client.query(
        "INSERT INTO client_contact_observation(account_id,branch_id,message_id,source_client_id,kind,value,original_text,status) VALUES($1,$2,$3,$4,'WHATSAPP','+5491155550505','Pedido','RECORDED')",
        [account, branch, message, routed],
      );
      // Two profiles each verified one form of a number: the first verified keeps it.
      const firstForm = randomUUID(),
        secondForm = randomUUID();
      await insertClient(firstForm, { name: 'First form' });
      await insertClient(secondForm, { name: 'Second form' });
      await client.query(
        "INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source,verified_at) VALUES($1,$2,'WHATSAPP','+541155550606',TRUE,'CHANNEL','2026-01-01'),($1,$3,'WHATSAPP','+5491155550606',TRUE,'CHANNEL','2026-02-01')",
        [account, firstForm, secondForm],
      );

      result = goose('up-to', 77);
      assert.equal(result.status, 0, stderr(result));

      const contacts = async (clientId) =>
        (
          await client.query(
            'SELECT value,source::text AS source,is_primary,verified_at IS NOT NULL AS verified FROM client_contact WHERE client_id=$1 ORDER BY kind,value',
            [clientId],
          )
        ).rows;
      assert.deepEqual(await contacts(typedOld), [
        { value: '+5491155550202', source: 'SELLER', is_primary: true, verified: false },
        { value: 'obra@example.test', source: 'SELLER', is_primary: true, verified: false },
      ]);
      assert.deepEqual(await contacts(channelNew), [
        { value: '+5491155550202', source: 'CHANNEL', is_primary: true, verified: true },
      ]);
      assert.deepEqual(await contacts(twin), [
        { value: '+5491155550303', source: 'CHANNEL', is_primary: true, verified: true },
      ]);
      assert.deepEqual(await contacts(registered), [
        { value: 'proved@example.test', source: 'CUSTOMER', is_primary: true, verified: true },
      ]);
      assert.deepEqual(await contacts(routed), [
        { value: '+5491155550505', source: 'SELLER', is_primary: true, verified: true },
      ]);
      assert.deepEqual(await contacts(firstForm), [
        { value: '+5491155550606', source: 'CHANNEL', is_primary: true, verified: true },
      ]);
      assert.deepEqual(await contacts(secondForm), [
        { value: '+5491155550606', source: 'CHANNEL', is_primary: true, verified: false },
      ]);
      assert.equal(
        (await client.query('SELECT phone FROM client WHERE id=$1', [typedOld])).rows[0].phone,
        '+5491155550202',
      );
      assert.deepEqual(
        (
          await client.query(
            "SELECT branch_id,source_client_id,candidate_client_id,value FROM client_contact_observation WHERE account_id=$1 AND status='PENDING' ORDER BY value",
            [account],
          )
        ).rows,
        [
          {
            branch_id: branch,
            source_client_id: typedOld,
            candidate_client_id: channelNew,
            value: '+5491155550202',
          },
          {
            branch_id: branch,
            source_client_id: secondForm,
            candidate_client_id: firstForm,
            value: '+5491155550606',
          },
        ],
      );
      assert.equal(
        (
          await client.query('SELECT value FROM client_contact_outbox WHERE account_id=$1', [
            account,
          ])
        ).rows[0].value,
        '+5491155550404',
      );
      const shape = async () =>
        (
          await client.query(
            "SELECT (SELECT column_default FROM information_schema.columns WHERE table_name='client_contact' AND column_name='verified_at') AS verified_default, EXISTS(SELECT 1 FROM pg_indexes WHERE indexname='idx_client_account_tax_id') AS redundant_index",
          )
        ).rows[0];
      assert.deepEqual(await shape(), { verified_default: null, redundant_index: false });

      result = goose('down-to', 76);
      assert.equal(result.status, 0, stderr(result));
      assert.deepEqual(await shape(), { verified_default: 'now()', redundant_index: true });
      result = goose('up-to', 77);
      assert.equal(result.status, 0, stderr(result));
    } finally {
      if (client) await client.end();
      await admin.query(`DROP DATABASE IF EXISTS ${name}`);
      await admin.end();
    }
  },
);
