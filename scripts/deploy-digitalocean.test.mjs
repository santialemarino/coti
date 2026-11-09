import assert from 'node:assert/strict';
import test from 'node:test';

import { deployProduction } from './deploy-digitalocean.mjs';

const sha = 'a'.repeat(40);
const coreId = '11111111-1111-4111-8111-111111111111';
const portalId = '22222222-2222-4222-8222-222222222222';
const environment = {
  GITHUB_ACTIONS: 'true',
  GITHUB_REF: 'refs/heads/main',
  GITHUB_SHA: sha,
  GITHUB_REPOSITORY: 'example/coti',
  DIGITALOCEAN_ACCESS_TOKEN: 'private-token',
  DIGITALOCEAN_CORE_APP_ID: coreId,
  DIGITALOCEAN_PORTAL_APP_ID: portalId,
  DO_DEPLOY_TIMEOUT_SECONDS: '2',
  DO_DEPLOY_POLL_SECONDS: '1',
};

function fixture({ failure, mismatch, stuck, healthStatus = 200 } = {}) {
  const source = { branch: 'main', repo_clone_url: 'https://github.com/example/coti.git' };
  const apps = new Map([
    [
      coreId,
      {
        spec: {
          name: 'coti-core',
          services: [{ name: 'api', git: source }],
          jobs: [{ name: 'migrate', kind: 'PRE_DEPLOY', git: source }],
          envs: [{ key: 'DATABASE_URL', type: 'SECRET', value: 'private-database' }],
        },
        active_deployment: { id: 'core-old', phase: 'ACTIVE' },
        default_ingress: 'https://core.test',
      },
    ],
    [
      portalId,
      {
        spec: { name: 'coti-portal', services: [{ name: 'webapp', git: source }] },
        active_deployment: { id: 'portal-old', phase: 'ACTIVE' },
        default_ingress: 'https://portal.test',
      },
    ],
  ]);
  const calls = [];
  let time = 0;
  const options = {
    env: { ...environment },
    log: () => {},
    now: () => time,
    sleep: async (ms) => {
      time += ms;
    },
    fetchImpl: async (input, init = {}) => {
      const url = new URL(input);
      calls.push({ path: url.pathname, host: url.host, method: init.method ?? 'GET', init });
      if (url.host !== 'api.digitalocean.com') {
        assert.equal(
          init.headers,
          undefined,
          'health probes must not receive the DigitalOcean token',
        );
        return new Response('', { status: healthStatus });
      }
      const id = url.pathname.split('/').at(-1);
      const app = apps.get(id);
      assert(app);
      if (init.method === 'PUT') {
        const body = JSON.parse(init.body);
        assert.equal(body.update_all_source_versions, true);
        assert.deepEqual(body.spec, app.spec, 'runtime configuration must remain unchanged');
        app.active_deployment = {
          id: id + '-new',
          phase: 'ACTIVE',
          services: app.spec.services.map((c) => ({
            name: c.name,
            source_commit_hash: mismatch ? 'b'.repeat(40) : sha,
          })),
          jobs: (app.spec.jobs ?? []).map((c) => ({ name: c.name, source_commit_hash: sha })),
        };
        if (failure) app.in_progress_deployment = { phase: 'ERROR' };
        if (stuck) app.in_progress_deployment = { phase: 'DEPLOYING' };
      }
      return new Response(JSON.stringify({ app: structuredClone(app) }), { status: 200 });
    },
  };
  return { apps, calls, options };
}

test('checks live configuration without issuing any mutation', async () => {
  const { calls, options } = fixture();
  options.env.GITHUB_REF = 'refs/heads/dev';
  await deployProduction({ ...options, checkOnly: true });
  assert.equal(calls.length, 2);
  assert(calls.every((c) => c.method === 'GET'));
});

test('deploys core and migrations before portal and preserves secrets without logging them', async () => {
  const { calls, options } = fixture();
  const logs = [];
  const results = await deployProduction({ ...options, log: (message) => logs.push(message) });
  assert.deepEqual(
    calls.filter((c) => c.method === 'PUT').map((c) => c.path),
    [`/v2/apps/${coreId}`, `/v2/apps/${portalId}`],
  );
  assert.equal(results.length, 2);
  assert.deepEqual(
    calls.filter((c) => c.host !== 'api.digitalocean.com').map((c) => c.path),
    ['/api/health', '/api/ready', '/manifest.webmanifest'],
  );
  assert(!logs.join('\n').includes('private-'));
});

test('refuses deployment from dev, PRs and local shells before contacting DigitalOcean', async () => {
  for (const overrides of [
    { GITHUB_REF: 'refs/heads/dev' },
    { GITHUB_REF: 'refs/pull/231/merge' },
    { GITHUB_ACTIONS: 'false' },
  ]) {
    const { calls, options } = fixture();
    await assert.rejects(
      deployProduction({ ...options, env: { ...options.env, ...overrides } }),
      /requires a main/,
    );
    assert.equal(calls.length, 0);
  }
});

test('validates both targets before updating either application', async () => {
  const { apps, calls, options } = fixture();
  apps.get(portalId).spec.services[0].git = {
    branch: 'dev',
    repo_clone_url: 'https://github.com/example/coti',
  };
  await assert.rejects(deployProduction(options), /expected repository on main/);
  assert(!calls.some((c) => c.method === 'PUT'));
});

test('refuses to overlap an existing deployment or bypass migrations', async () => {
  for (const change of [
    (app) => {
      app.in_progress_deployment = { phase: 'DEPLOYING' };
    },
    (app) => {
      app.spec.jobs = [];
    },
  ]) {
    const { apps, calls, options } = fixture();
    change(apps.get(coreId));
    await assert.rejects(deployProduction(options));
    assert(!calls.some((c) => c.method === 'PUT'));
  }
});

test('stops before portal when core fails, times out, runs another commit or is unhealthy', async () => {
  for (const behavior of [
    { failure: true },
    { stuck: true },
    { mismatch: true },
    { healthStatus: 503 },
  ]) {
    const { calls, options } = fixture(behavior);
    await assert.rejects(deployProduction(options));
    assert.equal(calls.filter((c) => c.method === 'PUT').length, 1);
  }
});

test('a provider error does not reveal its response body or credentials', async () => {
  const { options } = fixture();
  await assert.rejects(
    deployProduction({
      ...options,
      fetchImpl: async () => new Response('private-token private-database', { status: 403 }),
    }),
    (error) => error.message.includes('HTTP 403') && !error.message.includes('private-'),
  );
});
