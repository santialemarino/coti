import { spawnSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import pg from 'pg';

import {
  assertLocalDemoEnvironment,
  DEMO_ACCOUNT_ID,
  demoOptions,
  runDemoCommand,
} from './lib/demo-catalog.mjs';
import { loadAppUrl, loadOwnerUrl } from './lib/owner-url.mjs';

async function main() {
  const options = demoOptions(process.argv.slice(2));
  const ownerUrl = loadOwnerUrl();
  const appUrl = loadAppUrl();
  const localConfig = existsSync('apps/api/.env') ? readFileSync('apps/api/.env', 'utf8') : '';
  const environment = {
    ...process.env,
    ENV: process.env.ENV ?? localConfig.match(/^ENV\s*=\s*["']?([^\s"']+)/m)?.[1],
  };
  assertLocalDemoEnvironment(ownerUrl, environment);
  assertLocalDemoEnvironment(appUrl, environment);
  const owner = new URL(ownerUrl);
  const app = new URL(appUrl);
  if ((owner.port || '5432') !== (app.port || '5432') || owner.pathname !== app.pathname) {
    throw new Error('Demo owner and application roles must target the same local database.');
  }
  console.log(`Demo account: ${DEMO_ACCOUNT_ID}. Mode: ${options.apply ? 'apply' : 'dry-run'}.`);
  if (options.apply) {
    runDemoCommand(process.execPath, ['scripts/db-seed.mjs']);
    if (options.embeddings) {
      runDemoCommand('go', ['run', './cmd/catalog-embed', '--account', DEMO_ACCOUNT_ID], {
        cwd: 'apps/api',
        env: { ...process.env, DATABASE_URL: appUrl },
      });
      runDemoCommand(process.execPath, ['scripts/db-vector-index.mjs']);
    }
  }
  const client = new pg.Client({ connectionString: ownerUrl });
  await client.connect();
  try {
    const result = await client.query(
      `SELECT count(*)::int AS active_products,
      count(*) FILTER (WHERE embedding IS NULL OR embedding_updated_at IS NULL OR embedding_updated_at < updated_at)::int AS pending_embeddings
      FROM product WHERE account_id=$1 AND is_active`,
      [DEMO_ACCOUNT_ID],
    );
    console.log(JSON.stringify(result.rows[0]));
  } finally {
    await client.end();
  }
  if (options.benchmark) {
    if (!options.apply) {
      console.log('Dry run: benchmark would use the real provider; no provider was called.');
      return;
    }
    const commit = spawnSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' });
    runDemoCommand(
      'go',
      [
        'test',
        '-tags=integration',
        './internal/integration',
        '-run',
        '^TestDemoCatalogBenchmark$',
        '-count=1',
        '-timeout=10m',
        '-v',
      ],
      {
        cwd: 'apps/api',
        env: {
          ...process.env,
          TEST_DATABASE_URL: appUrl,
          TEST_DATABASE_ADMIN_URL: ownerUrl,
          COTI_DEMO_BENCHMARK: '1',
          COTI_BENCHMARK_COMMIT: commit.stdout?.trim() ?? 'unknown',
        },
      },
    );
  }
}

main().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});
