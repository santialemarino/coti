/**
 * Applies apps/api/database/02_seed_dev.sql as the owner role (so RLS doesn't apply).
 * Idempotent. Run from repo root: pnpm db:seed
 */
import { execSync } from 'child_process';
import fs from 'fs';
import path from 'path';

const ROOT = process.cwd();
const CONTAINER = 'coti-postgres';
const SEED_PATH = path.join(ROOT, 'apps/api/database/02_seed_dev.sql');

if (!fs.existsSync(SEED_PATH)) {
  console.error(`Seed file not found: ${SEED_PATH}`);
  process.exit(1);
}

console.log('Applying dev seed (02_seed_dev.sql)...');
execSync(`docker exec -i ${CONTAINER} psql -U coti -d coti -v ON_ERROR_STOP=1 -q`, {
  input: fs.readFileSync(SEED_PATH, 'utf8'),
  stdio: ['pipe', 'inherit', 'inherit'],
  cwd: ROOT,
});
console.log('Seed applied.');
