import fs from 'node:fs';
import path from 'node:path';

const CACHE_MILLISECONDS = 3_000;
const DEFAULT_API_URL = 'http://127.0.0.1:8001';
const DEFAULT_EMAIL = 'admin@corralonsanmartin.test';
const DEFAULT_PASSWORD = 'coti1234';
const DEFAULT_BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';

export function createRFQBranchManager({
  root,
  apiURL = process.env.RFQ_EVAL_API_URL ?? DEFAULT_API_URL,
  fetchImpl = fetch,
  environment = process.env,
}) {
  let cached = null;
  let cachedAt = 0;

  async function list({ force = false } = {}) {
    if (!force && cached && Date.now() - cachedAt < CACHE_MILLISECONDS) return cached;

    const env = { ...environment, ...readEnvFile(path.join(root, 'apps', 'api', '.env')) };
    const token = env.RFQ_EVAL_TOKEN || (await login(fetchImpl, apiURL, env));
    const response = await fetchImpl(`${apiURL.replace(/\/$/, '')}/v1/branches`, {
      headers: { Authorization: `Bearer ${token}` },
      signal: AbortSignal.timeout(3_000),
    });
    if (!response.ok) {
      throw new Error(`Branch listing returned HTTP ${response.status}`);
    }

    const payload = await response.json().catch(() => null);
    if (!Array.isArray(payload?.items)) throw new Error('Branch listing returned an invalid body');
    const items = payload.items
      .filter(
        (branch) =>
          branch?.is_active === true &&
          typeof branch.id === 'string' &&
          typeof branch.name === 'string',
      )
      .map((branch) => ({ id: branch.id, name: branch.name }));
    const configuredDefault = env.RFQ_EVAL_BRANCH_ID ?? DEFAULT_BRANCH_ID;
    cached = {
      items,
      default_branch_id: items.some((branch) => branch.id === configuredDefault)
        ? configuredDefault
        : (items[0]?.id ?? null),
    };
    cachedAt = Date.now();
    return cached;
  }

  return { list };
}

async function login(fetchImpl, apiURL, env) {
  const response = await fetchImpl(`${apiURL.replace(/\/$/, '')}/v1/public/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      email: env.RFQ_EVAL_EMAIL ?? DEFAULT_EMAIL,
      password: env.RFQ_EVAL_PASSWORD ?? DEFAULT_PASSWORD,
      remember_me: false,
    }),
    signal: AbortSignal.timeout(3_000),
  });
  if (!response.ok) throw new Error(`Branch authentication returned HTTP ${response.status}`);
  const payload = await response.json().catch(() => null);
  if (typeof payload?.access_token !== 'string') {
    throw new Error('Branch authentication returned an invalid body');
  }
  return payload.access_token;
}

function readEnvFile(file) {
  if (!fs.existsSync(file)) return {};
  const env = {};
  for (const line of fs.readFileSync(file, 'utf8').split(/\r?\n/)) {
    const match = line.match(/^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$/);
    if (!match) continue;
    let value = match[2].trim();
    if (
      (value.startsWith('"') && value.endsWith('"')) ||
      (value.startsWith("'") && value.endsWith("'"))
    ) {
      value = value.slice(1, -1);
    }
    env[match[1]] = value;
  }
  return env;
}
