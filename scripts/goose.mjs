/**
 * Wraps the goose CLI for migrations. Loads DATABASE_URL from apps/api/.env and
 * invokes goose pinned at GOOSE_VERSION via `go run`.
 *
 * Usage: node scripts/goose.mjs <command> [args]
 *   up | down | status | create <name> sql
 */
import { spawnSync } from 'child_process';
import fs from 'fs';
import path from 'path';

const GOOSE_VERSION = 'v3.27.1';
const ROOT = process.cwd();
const ENV_FILE = path.join(ROOT, 'apps/api/.env');
const MIGRATIONS_DIR = path.join(ROOT, 'apps/api/migrations');

function loadDatabaseUrl() {
  if (process.env.DATABASE_URL) return process.env.DATABASE_URL;
  if (!fs.existsSync(ENV_FILE)) {
    console.error(`DATABASE_URL not set and ${ENV_FILE} not found.`);
    process.exit(1);
  }
  const line = fs
    .readFileSync(ENV_FILE, 'utf8')
    .split('\n')
    .find((l) => l.startsWith('DATABASE_URL='));
  if (!line) {
    console.error(`DATABASE_URL not found in ${ENV_FILE}.`);
    process.exit(1);
  }
  return line.slice('DATABASE_URL='.length).trim().replace(/^["']|["']$/g, '');
}

const args = process.argv.slice(2);
if (args.length === 0) {
  console.error('Usage: node scripts/goose.mjs <command> [args]');
  process.exit(1);
}

const dbUrl = loadDatabaseUrl();
const gooseArgs = [
  'run',
  `github.com/pressly/goose/v3/cmd/goose@${GOOSE_VERSION}`,
  '-dir',
  MIGRATIONS_DIR,
  'postgres',
  dbUrl,
  ...args,
];

const result = spawnSync('go', gooseArgs, { stdio: 'inherit', cwd: ROOT });
process.exit(result.status ?? 1);
