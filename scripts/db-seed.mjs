/**
 * Applies apps/api/database/02_seed_dev.sql as the owner role (so RLS doesn't apply).
 * Idempotent. Run from repo root: pnpm db:seed
 *
 * It connects by DATABASE_ADMIN_URL rather than shelling into a container, so the seed reaches
 * whatever database that URL names — a differently named container, a Postgres installed on the
 * host, a remote development instance.
 */
import fs from 'fs';
import path from 'path';
import pg from 'pg';

import { loadOwnerUrl } from './lib/owner-url.mjs';

const ROOT = process.cwd();
const SEED_PATH = path.join(ROOT, 'apps/api/database/02_seed_dev.sql');

async function main() {
  if (!fs.existsSync(SEED_PATH)) {
    console.error(`Seed file not found: ${SEED_PATH}`);
    process.exit(1);
  }

  console.log('Applying dev seed (02_seed_dev.sql)...');
  const client = new pg.Client({ connectionString: loadOwnerUrl() });
  await client.connect();
  try {
    // The whole file in one call, which the driver runs as a single transaction: a seed that
    // fails halfway leaves nothing behind rather than a partial account for the next run to
    // trip over.
    await client.query(fs.readFileSync(SEED_PATH, 'utf8'));
  } finally {
    await client.end();
  }
  console.log('Seed applied.');
}

main().catch((error) => {
  console.error(error.message);
  process.exit(1);
});
