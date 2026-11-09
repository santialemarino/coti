import { appendFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

const applications = [
  ['coti-core', 'DIGITALOCEAN_CORE_APP_ID'],
  ['coti-portal', 'DIGITALOCEAN_PORTAL_APP_ID'],
];

function positiveInteger(value, fallback) {
  const result = Number(value ?? fallback);
  if (!Number.isSafeInteger(result) || result <= 0)
    throw new Error('Invalid deployment time limit');
  return result;
}

export function validateApplication(app, name, repository) {
  if (app.spec?.name !== name) throw new Error(`Unexpected application for ${name}`);
  if (app.in_progress_deployment) throw new Error(`${name} already has a deployment in progress`);
  const components = [...(app.spec.services ?? []), ...(app.spec.jobs ?? [])];
  if (components.length === 0) throw new Error(`${name} has no source components`);
  if (
    name === 'coti-core' &&
    !app.spec.jobs?.some((job) => job.name === 'migrate' && job.kind === 'PRE_DEPLOY')
  ) {
    throw new Error('Core requires its pre-deployment migration job');
  }
  for (const component of components) {
    const source = component.git ?? component.github;
    const repo =
      component.git?.repo_clone_url?.replace(/\.git$/, '') ??
      (component.github?.repo ? `https://github.com/${component.github.repo}` : '');
    if (source?.branch !== 'main' || repo !== `https://github.com/${repository}`) {
      throw new Error(`${name} must use the expected repository on main`);
    }
    if (component.github?.deploy_on_push)
      throw new Error(`${name} has competing automatic deployments`);
  }
}

export async function deployProduction({
  env = process.env,
  checkOnly = false,
  fetchImpl = fetch,
  sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
  now = Date.now,
  log = console.log,
} = {}) {
  if (
    !checkOnly &&
    (env.GITHUB_ACTIONS !== 'true' ||
      env.GITHUB_REF !== 'refs/heads/main' ||
      !/^[a-f0-9]{40}$/.test(env.GITHUB_SHA ?? ''))
  ) {
    throw new Error('Production deployment requires a main GitHub Actions run');
  }
  if (!env.DIGITALOCEAN_ACCESS_TOKEN || !/^[\w.-]+\/[\w.-]+$/.test(env.GITHUB_REPOSITORY ?? '')) {
    throw new Error('Missing deployment credentials or repository');
  }
  const timeout = positiveInteger(env.DO_DEPLOY_TIMEOUT_SECONDS, 1800) * 1000;
  const interval = positiveInteger(env.DO_DEPLOY_POLL_SECONDS, 10) * 1000;
  async function api(path, options = {}) {
    const response = await fetchImpl('https://api.digitalocean.com/v2' + path, {
      ...options,
      headers: {
        Authorization: `Bearer ${env.DIGITALOCEAN_ACCESS_TOKEN}`,
        'Content-Type': 'application/json',
      },
      signal: AbortSignal.timeout(30000),
    });
    if (!response.ok) throw new Error(`DigitalOcean request failed (HTTP ${response.status})`);
    try {
      return await response.json();
    } catch {
      throw new Error('DigitalOcean returned an invalid response');
    }
  }
  const apps = [];
  for (const [name, key] of applications) {
    const id = env[key];
    if (!/^[a-f0-9-]{36}$/.test(id ?? '')) throw new Error(`Missing or invalid ${key}`);
    const { app } = await api(`/apps/${id}`);
    validateApplication(app, name, env.GITHUB_REPOSITORY);
    apps.push({ id, name, app });
  }
  if (checkOnly) {
    log('Deployment configuration verified for both applications; no changes made');
    return [];
  }
  const results = [];
  for (const { id, name } of apps) {
    const { app } = await api(`/apps/${id}`);
    validateApplication(app, name, env.GITHUB_REPOSITORY);
    await api(`/apps/${id}`, {
      method: 'PUT',
      body: JSON.stringify({ spec: app.spec, update_all_source_versions: true }),
    });
    const deadline = now() + timeout;
    let ready;
    while (now() < deadline) {
      const { app: current } = await api(`/apps/${id}`);
      const pending = current.in_progress_deployment;
      if (['ERROR', 'CANCELED', 'SUPERSEDED'].includes(pending?.phase))
        throw new Error(`${name} deployment failed`);
      const active = current.active_deployment;
      if (!pending && active?.id !== app.active_deployment?.id && active?.phase === 'ACTIVE') {
        const components = [...(active.services ?? []), ...(active.jobs ?? [])];
        const expectedCount = (app.spec.services?.length ?? 0) + (app.spec.jobs?.length ?? 0);
        if (
          components.length !== expectedCount ||
          components.some((c) => c.source_commit_hash !== env.GITHUB_SHA)
        ) {
          throw new Error(`${name} deployed a different commit; a newer main run must complete`);
        }
        ready = current;
        break;
      }
      if (!pending && ['ERROR', 'CANCELED'].includes(current.last_deployment?.phase))
        throw new Error(`${name} deployment failed`);
      await sleep(interval);
    }
    if (!ready)
      throw new Error(`${name} deployment timed out; check DigitalOcean Activity before retrying`);
    const origin = new URL(ready.default_ingress || ready.live_url);
    if (origin.protocol !== 'https:' || origin.username || origin.password)
      throw new Error(`${name} requires HTTPS`);
    for (const path of name === 'coti-core'
      ? ['/api/health', '/api/ready']
      : ['/manifest.webmanifest']) {
      const response = await fetchImpl(new URL(path, origin), {
        redirect: 'manual',
        signal: AbortSignal.timeout(30000),
      });
      await response.body?.cancel();
      if (response.status !== 200)
        throw new Error(`${name} health check failed (HTTP ${response.status})`);
    }
    const result = { name, deployment: ready.active_deployment.id, commit: env.GITHUB_SHA };
    results.push(result);
    log(`${name} is active at ${env.GITHUB_SHA}`);
  }
  return results;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.slice(2).some((arg) => arg !== '--check'))
      throw new Error('Only --check is supported');
    const results = await deployProduction({ checkOnly: process.argv.includes('--check') });
    if (process.env.GITHUB_STEP_SUMMARY && results.length) {
      appendFileSync(
        process.env.GITHUB_STEP_SUMMARY,
        results.map((r) => `- ${r.name}: ${r.commit} (${r.deployment})\n`).join(''),
      );
    }
  } catch (error) {
    console.error(error instanceof Error ? error.message : 'Deployment failed');
    process.exitCode = 1;
  }
}
