/**
 * Creates the catalog's approximate vector index. Runs as the OWNER role (DATABASE_ADMIN_URL):
 * the index spans every account and the request role does not own the table. The logic lives in
 * lib/vector-index.mjs.
 *
 * It is deliberately not a migration: built on an empty table the index is degenerate, so it
 * belongs after the catalog is loaded and embedded (`go run ./cmd/catalog-embed`).
 * Usage: node scripts/db-vector-index.mjs [--lists <n>]
 */
import pg from 'pg';

import { loadOwnerUrl } from './lib/owner-url.mjs';
import { createVectorIndex, parseArgs } from './lib/vector-index.mjs';

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
    result = await createVectorIndex(client, args);
  } finally {
    await client.end();
  }

  if (!result.created) {
    fail('No product carries an embedding yet. Run `go run ./cmd/catalog-embed` first.');
  }
  console.log(`Indexed ${result.embedded} embedded product(s) with lists = ${result.lists}.`);
}

main().catch((err) => fail(err.stack ?? String(err)));
