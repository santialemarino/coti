/**
 * Deactivates or reactivates a corralón. Runs as the OWNER role (DATABASE_ADMIN_URL): it is not
 * request-scoped, so there is no tenant to narrow it to. The logic lives in lib/account-state.mjs.
 * Usage: node scripts/db-account.mjs <deactivate | activate> --account <uuid>
 */
import pg from 'pg';

import { parseArgs, setAccountActive } from './lib/account-state.mjs';
import { loadOwnerUrl } from './lib/owner-url.mjs';

function fail(message) {
  console.error(message);
  process.exit(1);
}

async function main() {
  let args;
  try {
    args = parseArgs(process.argv.slice(2));
  } catch (err) {
    fail(err.message);
  }

  const client = new pg.Client({ connectionString: loadOwnerUrl() });
  await client.connect();
  let result;
  try {
    result = await setAccountActive(client, args);
  } finally {
    await client.end();
  }

  if (result === null) fail(`No account with id ${args.id}. Nothing changed.`);

  const verb = args.activate ? 'Activated' : 'Deactivated';
  const before = result.wasActive ? 'active' : 'inactive';
  console.log(`${verb} "${result.name}" (${args.id}), previously ${before}.`);
  if (!args.activate) {
    console.log(`Bumped session_epoch for ${result.usersCut} user(s).`);
  }
}

main().catch((err) => fail(err.stack ?? String(err)));
