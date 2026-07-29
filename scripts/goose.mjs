/**
 * Wraps the goose CLI, pinned at GOOSE_VERSION via `go run`. Migrations run as the OWNER role
 * (DATABASE_ADMIN_URL), not the RLS-restricted app role, since they create and grant on tables.
 * Usage: node scripts/goose.mjs <up | down | status | create <name> sql>
 */
import { spawnSync } from 'child_process';
import fs from 'fs';
import path from 'path';

const GOOSE_VERSION = 'v3.27.1';
const ROOT = process.cwd();
const ENV_FILE = path.join(ROOT, 'apps/api/.env');
const MIGRATIONS_DIR = path.join(ROOT, 'apps/api/migrations');

const KEY = 'DATABASE_ADMIN_URL';

function loadOwnerUrl() {
  if (process.env[KEY]) return process.env[KEY];
  if (!fs.existsSync(ENV_FILE)) {
    console.error(`${KEY} not set and ${ENV_FILE} not found.`);
    process.exit(1);
  }
  const line = fs
    .readFileSync(ENV_FILE, 'utf8')
    .split('\n')
    .find((l) => l.startsWith(`${KEY}=`));
  if (!line) {
    console.error(`${KEY} not found in ${ENV_FILE}.`);
    process.exit(1);
  }
  return line
    .slice(`${KEY}=`.length)
    .trim()
    .replace(/^["']|["']$/g, '');
}

const args = process.argv.slice(2);
if (args.length === 0) {
  console.error('Usage: node scripts/goose.mjs <command> [args]');
  process.exit(1);
}

const dbUrl = loadOwnerUrl();

// -s keeps new migrations sequentially numbered (00003, 00004…). Without it goose
// timestamps them, which still orders correctly but reads inconsistently next to the
// ones already in the directory.
const flags = args[0] === 'create' ? ['-s'] : [];

const gooseArgs = [
  'run',
  `github.com/pressly/goose/v3/cmd/goose@${GOOSE_VERSION}`,
  '-dir',
  MIGRATIONS_DIR,
  ...flags,
  'postgres',
  dbUrl,
  ...args,
];

const result = spawnSync('go', gooseArgs, { stdio: 'inherit', cwd: ROOT });
process.exit(result.status ?? 1);
