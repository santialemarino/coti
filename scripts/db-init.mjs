/**
 * Starts Postgres (docker-compose) and brings the schema to head with goose. Migrations are the
 * only thing that writes to a database; apps/api/database/ is a read reference, never applied.
 * Run from repo root: pnpm db:init
 */
import { execSync } from 'child_process';

const ROOT = process.cwd();
const CONTAINER = 'coti-postgres';

function run(cmd, opts = {}) {
  return execSync(cmd, { stdio: 'inherit', cwd: ROOT, ...opts });
}

function isPostgresReady() {
  try {
    execSync(`docker exec ${CONTAINER} psql -U coti -d coti -c "SELECT 1"`, {
      stdio: 'pipe',
      cwd: ROOT,
    });
    return true;
  } catch {
    return false;
  }
}

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}

async function waitForPostgres(maxAttempts = 15) {
  for (let i = 0; i < maxAttempts; i++) {
    if (isPostgresReady()) return;
    if (i === 0) process.stdout.write('Waiting for Postgres');
    process.stdout.write('.');
    await sleep(i < 3 ? 500 : 1000);
  }
  console.error('\nPostgres did not become ready in time.');
  process.exit(1);
}

async function main() {
  console.log('Starting Postgres (docker compose up -d postgres)...');
  run('docker compose up -d postgres');

  await waitForPostgres();
  console.log(' Postgres is ready.');

  console.log('Applying migrations (goose up)...');
  run('node scripts/goose.mjs up');

  console.log('\nDatabase initialized.');
}

main();
