/**
 * Ensures Postgres (docker-compose) is up and applies the canonical schema.
 * Run from repo root: pnpm db:init
 */
import { execSync } from 'child_process';
import fs from 'fs';
import path from 'path';

const ROOT = process.cwd();
const CONTAINER = 'coti-postgres';
const SCHEMA_PATH = path.join(ROOT, 'apps/api/database/01_create_tables.sql');

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

  if (!fs.existsSync(SCHEMA_PATH) || fs.statSync(SCHEMA_PATH).size === 0) {
    console.log('\nNo canonical schema yet at apps/api/database/01_create_tables.sql.');
    console.log('Database container is up; use pnpm db:migrate once migrations exist.');
    return;
  }

  console.log('Applying canonical schema (01_create_tables.sql)...');
  const sql = fs.readFileSync(SCHEMA_PATH, 'utf8');
  execSync(`docker exec -i ${CONTAINER} psql -U coti -d coti`, {
    input: sql,
    stdio: ['pipe', 'inherit', 'inherit'],
    cwd: ROOT,
  });

  console.log('\nDatabase initialized.');
}

main();
