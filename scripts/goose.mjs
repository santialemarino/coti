/**
 * Wraps the goose CLI, pinned as a tool dependency in apps/api/go.mod. Migrations run as the
 * OWNER role (DATABASE_ADMIN_URL), not the RLS-restricted app role, since they create and grant
 * on tables. Usage: node scripts/goose.mjs <up | down | status | create <name> sql>
 */
import { spawnSync } from 'child_process';
import path from 'path';

import { loadOwnerUrl } from './lib/owner-url.mjs';

const ROOT = process.cwd();
const API_DIR = path.join(ROOT, 'apps/api');
const MIGRATIONS_DIR = path.join(API_DIR, 'migrations');

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

// Every migration here is plain SQL. goose defaults `create` to a Go migration, which
// compiles into the binary instead of living in the chain the way the rest do, so the
// type is appended unless the caller named one.
const isCreate = args[0] === 'create';
const type = isCreate && !['sql', 'go'].includes(args.at(-1)) ? ['sql'] : [];

const gooseArgs = [
  'tool',
  'goose',
  '-dir',
  MIGRATIONS_DIR,
  ...flags,
  'postgres',
  dbUrl,
  ...args,
  ...type,
];

// `go tool` reads the pin from the module, so the command runs in apps/api. -dir is absolute
// and unaffected.
const result = spawnSync('go', gooseArgs, { stdio: 'inherit', cwd: API_DIR });
process.exit(result.status ?? 1);
