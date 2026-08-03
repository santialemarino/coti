/**
 * Flips account.is_active, which login, refresh and tenant resolution all read on every
 * request. Runs as the OWNER role (DATABASE_ADMIN_URL): it is not request-scoped, so there is
 * no tenant to narrow it to.
 * Usage: node scripts/db-account.mjs <deactivate | activate> --account <uuid>
 */
import pg from 'pg';

import { loadOwnerUrl } from './lib/owner-url.mjs';

const USAGE = 'Usage: node scripts/db-account.mjs <deactivate | activate> --account <uuid>';
const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function fail(message) {
  console.error(message);
  process.exit(1);
}

function parseArgs(argv) {
  const [action, ...rest] = argv;
  if (action !== 'deactivate' && action !== 'activate') fail(USAGE);

  const flag = rest.indexOf('--account');
  const id = flag === -1 ? '' : rest[flag + 1];
  if (!id || !UUID_PATTERN.test(id)) fail(`--account takes an account uuid.\n${USAGE}`);

  return { activate: action === 'activate', id };
}

/** Applies the flag in one transaction, returning what it found or null for an unknown id. */
async function apply(client, { activate, id }) {
  await client.query('BEGIN');
  try {
    // FOR UPDATE so two runs against the same account serialize rather than both reporting
    // the state they read.
    const found = await client.query(
      'SELECT name, is_active FROM account WHERE id = $1 FOR UPDATE',
      [id],
    );
    if (found.rowCount === 0) {
      await client.query('ROLLBACK');
      return null;
    }

    await client.query('UPDATE account SET is_active = $2 WHERE id = $1', [id, activate]);

    let usersCut = 0;
    if (!activate) {
      // The access tokens the users already hold die here rather than lasting out their
      // remaining lifetime. Activating needs no counterpart: the account is read again on
      // every request, so access returns on its own.
      const bumped = await client.query(
        'UPDATE app_user SET session_epoch = session_epoch + 1 WHERE account_id = $1',
        [id],
      );
      usersCut = bumped.rowCount;
    }

    await client.query('COMMIT');
    return { name: found.rows[0].name, wasActive: found.rows[0].is_active, usersCut };
  } catch (err) {
    await client.query('ROLLBACK').catch(() => {});
    throw err;
  }
}

async function main() {
  const args = parseArgs(process.argv.slice(2));

  const client = new pg.Client({ connectionString: loadOwnerUrl() });
  await client.connect();
  let result;
  try {
    result = await apply(client, args);
  } finally {
    await client.end();
  }

  if (result === null) fail(`No account with id ${args.id}. Nothing changed.`);

  const verb = args.activate ? 'Activated' : 'Deactivated';
  const before = result.wasActive ? 'active' : 'inactive';
  console.log(`${verb} "${result.name}" (${args.id}), previously ${before}.`);
  if (!args.activate) {
    console.log(
      `Bumped session_epoch for ${result.usersCut} user(s): their access tokens are dead.`,
    );
  }
}

main().catch((err) => fail(err.stack ?? String(err)));
