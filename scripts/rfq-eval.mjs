import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

import { writeRFQDashboard } from './lib/rfq-report.mjs';

const ROOT = process.cwd();
const DEFAULT_CASES = path.join(ROOT, 'scripts', 'fixtures', 'rfq-eval-cases.json');
const DEFAULT_API_URL = 'http://127.0.0.1:8000';
const DEFAULT_EMAIL = 'admin@corralonsanmartin.test';
const DEFAULT_PASSWORD = 'coti1234';
const DEFAULT_BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const DEFAULT_FROM = '+5491100000000';
const DEFAULT_PROFILE_NAME = 'RFQ evaluation';

export function parseArgs(args) {
  const options = { price: false, verbose: false };
  for (let i = 0; i < args.length; i += 1) {
    const arg = args[i];
    if (arg === '--price') {
      options.price = true;
      continue;
    }
    if (arg === '--verbose') {
      options.verbose = true;
      continue;
    }
    if (arg === '--help' || arg === '-h') {
      options.help = true;
      continue;
    }
    const key = {
      '--api-url': 'apiURL',
      '--branch': 'branchID',
      '--case': 'caseID',
      '--cases': 'casesPath',
      '--channel': 'channelID',
      '--report': 'reportPath',
    }[arg];
    if (!key || !args[i + 1]) {
      throw new Error(`Unknown or incomplete argument: ${arg}`);
    }
    options[key] = args[i + 1];
    i += 1;
  }
  return options;
}

export function validateCases(value) {
  if (!Array.isArray(value) || value.length === 0) {
    throw new Error('The evaluation file must contain a non-empty JSON array');
  }
  const ids = new Set();
  for (const [index, entry] of value.entries()) {
    if (!entry || typeof entry !== 'object') {
      throw new Error(`Case ${index} must be an object`);
    }
    if (typeof entry.id !== 'string' || entry.id.trim() === '') {
      throw new Error(`Case ${index} needs a non-empty id`);
    }
    if (ids.has(entry.id)) {
      throw new Error(`Duplicate case id: ${entry.id}`);
    }
    ids.add(entry.id);
    if (typeof entry.message !== 'string' || entry.message.trim() === '') {
      throw new Error(`Case ${entry.id} needs a non-empty message`);
    }
    if (!entry.expected || typeof entry.expected !== 'object') {
      throw new Error(`Case ${entry.id} needs expected assertions`);
    }
  }
  return value;
}

export function evaluateDraft(expected, status, body) {
  const failures = [];
  const expectedStatus = expected.http_status ?? 201;
  if (status !== expectedStatus) {
    failures.push(`HTTP status ${status}, want ${expectedStatus}`);
    return failures;
  }

  if (expected.rfq_status !== undefined && body?.rfq?.status !== expected.rfq_status) {
    failures.push(`rfq.status ${show(body?.rfq?.status)}, want ${show(expected.rfq_status)}`);
  }

  const quoteStatus = body?.quote?.current_status ?? null;
  if (expected.quote_status !== undefined && quoteStatus !== expected.quote_status) {
    failures.push(`quote.current_status ${show(quoteStatus)}, want ${show(expected.quote_status)}`);
  }

  const items = Array.isArray(body?.items) ? body.items : [];
  if (expected.item_count !== undefined && items.length !== expected.item_count) {
    failures.push(`items.length ${items.length}, want ${expected.item_count}`);
  }
  for (const [index, wanted] of (expected.items ?? []).entries()) {
    const item = items[index];
    if (!item) {
      failures.push(`items[${index}] is missing`);
      continue;
    }
    compareItem(failures, item, wanted, index);
  }
  return failures;
}

export function evaluatePricing(expected, status, body) {
  const failures = [];
  if (status !== 200) {
    failures.push(`pricing HTTP status ${status}, want 200`);
    return failures;
  }
  if (
    expected.priced_quote_status !== undefined &&
    body?.quote?.current_status !== expected.priced_quote_status
  ) {
    failures.push(
      `priced quote.current_status ${show(body?.quote?.current_status)}, want ${show(expected.priced_quote_status)}`,
    );
  }
  if (expected.total_nonzero === true && Number(body?.version?.total ?? 0) <= 0) {
    failures.push(`priced version.total ${show(body?.version?.total)}, want a positive value`);
  }
  for (const [index, wanted] of (expected.priced_items ?? []).entries()) {
    const item = body?.items?.[index];
    if (!item) {
      failures.push(`priced items[${index}] is missing`);
      continue;
    }
    compareItem(failures, item, wanted, index, 'priced items');
  }
  return failures;
}

function compareItem(failures, item, expected, index, label = 'items') {
  if (
    expected.description_contains !== undefined &&
    !includesText(item.requested_description, expected.description_contains)
  ) {
    failures.push(
      `${label}[${index}].requested_description ${show(item.requested_description)} does not contain ${show(expected.description_contains)}`,
    );
  }
  if (expected.quantity !== undefined && item.quantity !== expected.quantity) {
    failures.push(
      `${label}[${index}].quantity ${show(item.quantity)}, want ${show(expected.quantity)}`,
    );
  }
  if (expected.unit_contains !== undefined && !includesText(item.unit, expected.unit_contains)) {
    failures.push(
      `${label}[${index}].unit ${show(item.unit)} does not contain ${show(expected.unit_contains)}`,
    );
  }
  if (expected.match_status !== undefined && item.match_status !== expected.match_status) {
    failures.push(
      `${label}[${index}].match_status ${show(item.match_status)}, want ${show(expected.match_status)}`,
    );
  }
  if (
    expected.rationale_present === true &&
    (typeof item.quantity_rationale !== 'string' || item.quantity_rationale.trim() === '')
  ) {
    failures.push(`${label}[${index}].quantity_rationale is empty`);
  }
  if (
    expected.pricing_unavailable !== undefined &&
    item.pricing_unavailable !== expected.pricing_unavailable
  ) {
    failures.push(
      `${label}[${index}].pricing_unavailable ${show(item.pricing_unavailable)}, want ${show(expected.pricing_unavailable)}`,
    );
  }
}

function includesText(value, expected) {
  return (
    typeof value === 'string' &&
    value.toLocaleLowerCase('es-AR').includes(expected.toLocaleLowerCase('es-AR'))
  );
}

function show(value) {
  return JSON.stringify(value);
}

function loadLocalEnv() {
  const envPath = path.join(ROOT, '.env');
  if (fs.existsSync(envPath)) {
    process.loadEnvFile(envPath);
  }
}

function usage() {
  return `Usage: pnpm eval:rfq [options]

Runs the versioned RFQ cases through the development WhatsApp endpoint.

Options:
  --api-url <url>    API base URL (default: ${DEFAULT_API_URL})
  --branch <uuid>    Active branch (default: seeded Villa Bosch branch)
  --case <id>        Run only one case from the evaluation file
  --channel <uuid>   WhatsApp channel; omit when the branch has exactly one
  --cases <path>     Evaluation JSON (default: scripts/fixtures/rfq-eval-cases.json)
  --report <path>    JSON report path (default: .artifacts/rfq-eval/<timestamp>.json)
  --price            Accept materials and inspect pricing for every draft
  --verbose          Print every API response as JSON
  --help             Show this help

Environment:
  RFQ_EVAL_TOKEN, or RFQ_EVAL_EMAIL and RFQ_EVAL_PASSWORD for automatic login
  RFQ_EVAL_API_URL, RFQ_EVAL_BRANCH_ID, RFQ_EVAL_CHANNEL_ID
  RFQ_EVAL_FROM, RFQ_EVAL_PROFILE_NAME, RFQ_EVAL_TIMEOUT_SECONDS`;
}

function settings(options) {
  const timeoutSeconds = Number(process.env.RFQ_EVAL_TIMEOUT_SECONDS ?? 35);
  if (!Number.isFinite(timeoutSeconds) || timeoutSeconds <= 0) {
    throw new Error('RFQ_EVAL_TIMEOUT_SECONDS must be a positive number');
  }
  const stamp = new Date().toISOString().replaceAll(':', '-');
  return {
    apiURL: (options.apiURL ?? process.env.RFQ_EVAL_API_URL ?? DEFAULT_API_URL).replace(/\/$/, ''),
    branchID: options.branchID ?? process.env.RFQ_EVAL_BRANCH_ID ?? DEFAULT_BRANCH_ID,
    channelID: options.channelID ?? process.env.RFQ_EVAL_CHANNEL_ID,
    casesPath: path.resolve(options.casesPath ?? process.env.RFQ_EVAL_CASES ?? DEFAULT_CASES),
    reportPath: path.resolve(
      options.reportPath ??
        process.env.RFQ_EVAL_REPORT ??
        path.join(ROOT, '.artifacts', 'rfq-eval', `${stamp}.json`),
    ),
    email: process.env.RFQ_EVAL_EMAIL ?? DEFAULT_EMAIL,
    password: process.env.RFQ_EVAL_PASSWORD ?? DEFAULT_PASSWORD,
    token: process.env.RFQ_EVAL_TOKEN,
    from: process.env.RFQ_EVAL_FROM ?? DEFAULT_FROM,
    profileName: process.env.RFQ_EVAL_PROFILE_NAME ?? DEFAULT_PROFILE_NAME,
    timeoutMs: timeoutSeconds * 1000,
    price: options.price,
    verbose: options.verbose,
  };
}

async function requestJSON(url, { body, branchID, timeoutMs, token }) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers.Authorization = `Bearer ${token}`;
  if (branchID) headers['X-Branch-Id'] = branchID;

  const response = await fetch(url, {
    method: 'POST',
    headers,
    body: JSON.stringify(body ?? {}),
    signal: AbortSignal.timeout(timeoutMs),
  });
  const raw = await response.text();
  let payload = null;
  if (raw !== '') {
    try {
      payload = JSON.parse(raw);
    } catch {
      payload = { raw };
    }
  }
  return { status: response.status, body: payload };
}

async function accessToken(cfg) {
  if (cfg.token) return cfg.token;
  const response = await requestJSON(`${cfg.apiURL}/v1/public/auth/login`, {
    body: { email: cfg.email, password: cfg.password, remember_me: false },
    timeoutMs: cfg.timeoutMs,
  });
  if (response.status !== 200 || typeof response.body?.access_token !== 'string') {
    throw new Error(`Login failed (${response.status}): ${JSON.stringify(response.body)}`);
  }
  return response.body.access_token;
}

async function runCase(entry, cfg, token) {
  const started = performance.now();
  const request = {
    from: entry.from ?? cfg.from,
    profile_name: entry.profile_name ?? cfg.profileName,
    text: entry.message,
  };
  if (entry.channel_id ?? cfg.channelID) request.channel_id = entry.channel_id ?? cfg.channelID;

  try {
    const draft = await requestJSON(`${cfg.apiURL}/v1/dev/whatsapp/messages`, {
      body: request,
      branchID: cfg.branchID,
      timeoutMs: cfg.timeoutMs,
      token,
    });
    const failures = evaluateDraft(entry.expected, draft.status, draft.body);
    let pricing = null;
    if ((cfg.price || entry.price_after_draft) && draft.body?.quote?.id) {
      pricing = await requestJSON(
        `${cfg.apiURL}/v1/quotes/${draft.body.quote.id}/accept-materials`,
        { branchID: cfg.branchID, timeoutMs: cfg.timeoutMs, token },
      );
      failures.push(...evaluatePricing(entry.expected, pricing.status, pricing.body));
    }
    return {
      id: entry.id,
      description: entry.description,
      message: entry.message,
      definition: entry,
      passed: failures.length === 0,
      duration_ms: Math.round(performance.now() - started),
      failures,
      draft,
      pricing,
    };
  } catch (error) {
    return {
      id: entry.id,
      description: entry.description,
      message: entry.message,
      definition: entry,
      passed: false,
      duration_ms: Math.round(performance.now() - started),
      failures: [error instanceof Error ? error.message : String(error)],
      draft: null,
      pricing: null,
    };
  }
}

function printResult(result, verbose) {
  const status = result.passed ? 'PASS' : 'FAIL';
  const items = result.draft?.body?.items?.length ?? '-';
  console.log(
    `${status.padEnd(4)}  ${result.id.padEnd(24)} ${String(result.duration_ms).padStart(6)} ms  items=${items}`,
  );
  for (const failure of result.failures) console.log(`      ${failure}`);
  if (verbose) {
    console.log(JSON.stringify({ draft: result.draft, pricing: result.pricing }, null, 2));
  }
}

async function main() {
  loadLocalEnv();
  const options = parseArgs(process.argv.slice(2));
  if (options.help) {
    console.log(usage());
    return;
  }
  const cfg = settings(options);
  const allCases = validateCases(JSON.parse(fs.readFileSync(cfg.casesPath, 'utf8')));
  const cases = options.caseID ? allCases.filter((entry) => entry.id === options.caseID) : allCases;
  if (cases.length === 0) {
    throw new Error(`No evaluation case has id ${show(options.caseID)}`);
  }
  const token = await accessToken(cfg);
  const startedAt = new Date();
  const results = [];

  console.log(`RFQ evaluation: ${cases.length} cases against ${cfg.apiURL}`);
  for (const entry of cases) {
    const result = await runCase(entry, cfg, token);
    results.push(result);
    printResult(result, cfg.verbose);
  }

  const passed = results.filter((result) => result.passed).length;
  const report = {
    started_at: startedAt.toISOString(),
    finished_at: new Date().toISOString(),
    api_url: cfg.apiURL,
    branch_id: cfg.branchID,
    cases_file: path.relative(ROOT, cfg.casesPath),
    summary: { total: results.length, passed, failed: results.length - passed },
    results,
  };
  fs.mkdirSync(path.dirname(cfg.reportPath), { recursive: true });
  fs.writeFileSync(cfg.reportPath, `${JSON.stringify(report, null, 2)}\n`);
  const dashboardPath = cfg.reportPath.replace(/\.json$/i, '') + '.html';
  writeRFQDashboard(report, cases, dashboardPath);

  console.log(`\nSummary: ${passed}/${results.length} passed`);
  console.log(`Report: ${path.relative(ROOT, cfg.reportPath)}`);
  console.log(`Dashboard: ${path.relative(ROOT, dashboardPath)}`);
  if (passed !== results.length) process.exitCode = 1;
}

const entrypoint = process.argv[1] ? pathToFileURL(path.resolve(process.argv[1])).href : '';
if (import.meta.url === entrypoint) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  });
}
