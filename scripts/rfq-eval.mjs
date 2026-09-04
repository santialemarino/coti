import { randomUUID } from 'node:crypto';
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
  const options = { price: false, trace: false, verbose: false };
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
    if (arg === '--trace') {
      options.trace = true;
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
      '--wait-seconds': 'waitSeconds',
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
  return evaluateDraftChecks(expected, status, body).map((check) => check.message);
}

export function evaluateDraftChecks(expected, status, body) {
  const failures = [];
  const expectedStatus = expected.http_status ?? 201;
  if (status !== expectedStatus) {
    failures.push(
      checkFailure('whatsapp_endpoint', `HTTP status ${status}, want ${expectedStatus}`),
    );
    return failures;
  }

  if (expected.rfq_status !== undefined && body?.rfq?.status !== expected.rfq_status) {
    failures.push(
      checkFailure(
        'rfq_persistence',
        `rfq.status ${show(body?.rfq?.status)}, want ${show(expected.rfq_status)}`,
      ),
    );
  }

  const quoteStatus = body?.quote?.current_status ?? null;
  if (expected.quote_status !== undefined && quoteStatus !== expected.quote_status) {
    failures.push(
      checkFailure(
        'draft_persistence',
        `quote.current_status ${show(quoteStatus)}, want ${show(expected.quote_status)}`,
      ),
    );
  }

  const items = Array.isArray(body?.items) ? body.items : [];
  if (expected.item_count !== undefined && items.length !== expected.item_count) {
    failures.push(
      checkFailure('ai_extraction', `items.length ${items.length}, want ${expected.item_count}`),
    );
  }
  for (const [index, wanted] of (expected.items ?? []).entries()) {
    const item = items[index];
    if (!item) {
      failures.push(checkFailure('ai_extraction', `items[${index}] is missing`));
      continue;
    }
    compareItem(failures, item, wanted, index);
  }
  return failures;
}

export function evaluatePricing(expected, status, body) {
  return evaluatePricingChecks(expected, status, body).map((check) => check.message);
}

export function evaluatePricingChecks(expected, status, body) {
  const failures = [];
  if (status !== 200) {
    failures.push(checkFailure('pricing', `pricing HTTP status ${status}, want 200`));
    return failures;
  }
  if (
    expected.priced_quote_status !== undefined &&
    body?.quote?.current_status !== expected.priced_quote_status
  ) {
    failures.push(
      checkFailure(
        'pricing',
        `priced quote.current_status ${show(body?.quote?.current_status)}, want ${show(expected.priced_quote_status)}`,
      ),
    );
  }
  if (expected.total_nonzero === true && Number(body?.version?.total ?? 0) <= 0) {
    failures.push(
      checkFailure(
        'pricing',
        `priced version.total ${show(body?.version?.total)}, want a positive value`,
      ),
    );
  }
  for (const [index, wanted] of (expected.priced_items ?? []).entries()) {
    const item = body?.items?.[index];
    if (!item) {
      failures.push(checkFailure('pricing', `priced items[${index}] is missing`));
      continue;
    }
    compareItem(failures, item, wanted, index, 'priced items');
  }
  return failures;
}

function compareItem(failures, item, expected, index, label = 'items') {
  const stage = label === 'items' ? 'ai_extraction' : 'pricing';
  if (
    expected.description_contains !== undefined &&
    !includesText(item.requested_description, expected.description_contains)
  ) {
    failures.push(
      checkFailure(
        stage,
        `${label}[${index}].requested_description ${show(item.requested_description)} does not contain ${show(expected.description_contains)}`,
      ),
    );
  }
  if (expected.quantity !== undefined && item.quantity !== expected.quantity) {
    failures.push(
      checkFailure(
        stage,
        `${label}[${index}].quantity ${show(item.quantity)}, want ${show(expected.quantity)}`,
      ),
    );
  }
  if (expected.unit_contains !== undefined && !includesText(item.unit, expected.unit_contains)) {
    failures.push(
      checkFailure(
        stage,
        `${label}[${index}].unit ${show(item.unit)} does not contain ${show(expected.unit_contains)}`,
      ),
    );
  }
  if (expected.match_status !== undefined && item.match_status !== expected.match_status) {
    failures.push(
      checkFailure(
        label === 'items' ? 'catalog_matching' : 'pricing',
        `${label}[${index}].match_status ${show(item.match_status)}, want ${show(expected.match_status)}`,
      ),
    );
  }
  if (
    expected.rationale_present === true &&
    (typeof item.quantity_rationale !== 'string' || item.quantity_rationale.trim() === '')
  ) {
    failures.push(checkFailure(stage, `${label}[${index}].quantity_rationale is empty`));
  }
  if (
    expected.pricing_unavailable !== undefined &&
    item.pricing_unavailable !== expected.pricing_unavailable
  ) {
    failures.push(
      checkFailure(
        'pricing',
        `${label}[${index}].pricing_unavailable ${show(item.pricing_unavailable)}, want ${show(expected.pricing_unavailable)}`,
      ),
    );
  }
}

function checkFailure(stage, message) {
  return { stage, message };
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
  --wait-seconds <n> Wait for the API before starting (default: 0)
  --price            Accept materials and inspect pricing for every draft
  --trace            Print and store the execution timeline
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
  const waitSeconds = Number(options.waitSeconds ?? process.env.RFQ_EVAL_WAIT_SECONDS ?? 0);
  if (!Number.isFinite(waitSeconds) || waitSeconds < 0) {
    throw new Error('RFQ_EVAL_WAIT_SECONDS must be a non-negative number');
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
    waitMs: waitSeconds * 1000,
    price: options.price,
    trace: options.trace,
    verbose: options.verbose,
  };
}

async function requestJSON(
  url,
  { body, branchID, method = 'POST', requestID = randomUUID(), timeoutMs, token },
) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers.Authorization = `Bearer ${token}`;
  if (branchID) headers['X-Branch-Id'] = branchID;
  headers['X-Request-Id'] = requestID;

  const response = await fetch(url, {
    method,
    headers,
    body: method === 'GET' || method === 'HEAD' ? undefined : JSON.stringify(body ?? {}),
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
  return {
    status: response.status,
    body: payload,
    request_id: response.headers.get('x-request-id') ?? requestID,
  };
}

export function createTraceRecorder({ enabled = false, write = console.log } = {}) {
  const events = [];
  const color = enabled && process.stdout.isTTY && !process.env.NO_COLOR;
  const paint = (code, value) => (color ? `\u001b[${code}m${value}\u001b[0m` : value);
  const labels = {
    RUNNING: paint('36', 'RUN '),
    PASSED: paint('32', 'OK  '),
    FAILED: paint('31', 'FAIL'),
    SKIPPED: paint('33', 'SKIP'),
  };

  function print(event) {
    if (!enabled) return;
    const duration = event.duration_ms === undefined ? '' : ` ${event.duration_ms} ms`;
    const detail = event.detail ? ` - ${event.detail}` : '';
    write(`  ${labels[event.status]}  ${event.label}${duration}${detail}`);
  }

  return {
    events,
    observe(stage, label, status, detail, extra = {}) {
      const event = { stage, label, status, duration_ms: 0, detail, ...extra };
      events.push(event);
      print(event);
      return event;
    },
    start(stage, label, detail) {
      const event = { stage, label, status: 'RUNNING', detail };
      const started = performance.now();
      events.push(event);
      print(event);
      return (status, finishedDetail, extra = {}) => {
        event.status = status;
        event.duration_ms = Math.round(performance.now() - started);
        event.detail = finishedDetail;
        Object.assign(event, extra);
        print(event);
        return event;
      };
    },
  };
}

async function waitForAPI(cfg, trace) {
  const finish = trace.start('api_health', 'API health check', cfg.apiURL);
  const deadline = Date.now() + cfg.waitMs;
  let lastError;
  do {
    try {
      const response = await requestJSON(`${cfg.apiURL}/health`, {
        method: 'GET',
        timeoutMs: Math.min(cfg.timeoutMs, 2000),
      });
      if (response.status === 200) {
        finish('PASSED', `HTTP 200 at ${cfg.apiURL}`, { request_id: response.request_id });
        return;
      }
      lastError = new Error(`health endpoint returned HTTP ${response.status}`);
    } catch (error) {
      lastError = error;
    }
    if (Date.now() < deadline) await new Promise((resolve) => setTimeout(resolve, 250));
  } while (Date.now() < deadline);

  const message = lastError instanceof Error ? lastError.message : String(lastError);
  finish('FAILED', message);
  throw new Error(`API is not ready at ${cfg.apiURL}: ${message}`);
}

async function accessToken(cfg, trace) {
  if (cfg.token) {
    trace.observe('authentication', 'Authentication', 'PASSED', 'Using RFQ_EVAL_TOKEN');
    return cfg.token;
  }
  const finish = trace.start('authentication', 'Seeded user login', cfg.email);
  const response = await requestJSON(`${cfg.apiURL}/v1/public/auth/login`, {
    body: { email: cfg.email, password: cfg.password, remember_me: false },
    timeoutMs: cfg.timeoutMs,
  });
  if (response.status !== 200 || typeof response.body?.access_token !== 'string') {
    finish('FAILED', `HTTP ${response.status}`, { request_id: response.request_id });
    throw new Error(`Login failed (${response.status}): ${JSON.stringify(response.body)}`);
  }
  finish('PASSED', `HTTP ${response.status}`, { request_id: response.request_id });
  return response.body.access_token;
}

function recordDraftStages(trace, draft, checks) {
  const items = Array.isArray(draft.body?.items) ? draft.body.items : [];
  const failed = (stage) => checks.filter((check) => check.stage === stage);
  const record = (stage, label, detail) => {
    const stageFailures = failed(stage);
    trace.observe(
      stage,
      label,
      stageFailures.length === 0 ? 'PASSED' : 'FAILED',
      stageFailures.map((check) => check.message).join('; ') || detail,
    );
  };

  record(
    'rfq_persistence',
    'RFQ reception and persistence',
    `RFQ ${draft.body?.rfq?.id ?? 'missing'}`,
  );
  record('ai_extraction', 'AI material extraction', `${items.length} item(s) returned`);
  if (items.length === 0) {
    trace.observe('catalog_matching', 'Catalog matching', 'SKIPPED', 'No extracted items');
    trace.observe('draft_persistence', 'Quote draft persistence', 'SKIPPED', 'No extracted items');
    return;
  }
  const statuses = items.reduce((counts, item) => {
    const status = item.match_status ?? 'UNKNOWN';
    counts[status] = (counts[status] ?? 0) + 1;
    return counts;
  }, {});
  record('catalog_matching', 'Catalog matching', JSON.stringify(statuses));
  record(
    'draft_persistence',
    'Quote draft persistence',
    `quote=${draft.body?.quote?.id ?? 'missing'}, version=${draft.body?.version?.id ?? 'missing'}`,
  );
}

async function runCase(entry, cfg, token) {
  const started = performance.now();
  const trace = createTraceRecorder({ enabled: cfg.trace });
  let activeFinish = null;
  const request = {
    from: entry.from ?? cfg.from,
    profile_name: entry.profile_name ?? cfg.profileName,
    text: entry.message,
  };
  if (entry.channel_id ?? cfg.channelID) request.channel_id = entry.channel_id ?? cfg.channelID;

  try {
    if (cfg.trace) console.log(`\n${entry.id}: ${entry.description ?? entry.message}`);
    trace.observe('case_input', 'WhatsApp fixture', 'PASSED', entry.message);
    activeFinish = trace.start(
      'whatsapp_endpoint',
      'POST /v1/dev/whatsapp/messages',
      'Waiting for extraction, matching and persistence',
    );
    const draft = await requestJSON(`${cfg.apiURL}/v1/dev/whatsapp/messages`, {
      body: request,
      branchID: cfg.branchID,
      timeoutMs: cfg.timeoutMs,
      token,
    });
    const draftChecks = evaluateDraftChecks(entry.expected, draft.status, draft.body);
    activeFinish(
      draft.status === (entry.expected.http_status ?? 201) ? 'PASSED' : 'FAILED',
      `HTTP ${draft.status}`,
      { request_id: draft.request_id },
    );
    activeFinish = null;
    if (draft.status === (entry.expected.http_status ?? 201)) {
      recordDraftStages(trace, draft, draftChecks);
    }
    const failures = draftChecks.map((check) => check.message);
    let pricing = null;
    if ((cfg.price || entry.price_after_draft) && draft.body?.quote?.id) {
      activeFinish = trace.start(
        'pricing',
        'POST /accept-materials',
        'Deterministic price calculation',
      );
      pricing = await requestJSON(
        `${cfg.apiURL}/v1/quotes/${draft.body.quote.id}/accept-materials`,
        { branchID: cfg.branchID, timeoutMs: cfg.timeoutMs, token },
      );
      const pricingChecks = evaluatePricingChecks(entry.expected, pricing.status, pricing.body);
      failures.push(...pricingChecks.map((check) => check.message));
      activeFinish(pricingChecks.length === 0 ? 'PASSED' : 'FAILED', `HTTP ${pricing.status}`, {
        request_id: pricing.request_id,
      });
      activeFinish = null;
    } else {
      trace.observe('pricing', 'Material acceptance and pricing', 'SKIPPED', 'Not requested');
    }
    trace.observe(
      'contract_validation',
      'Expected response assertions',
      failures.length === 0 ? 'PASSED' : 'FAILED',
      failures.length === 0 ? 'All declared expectations passed' : failures.join('; '),
    );
    return {
      id: entry.id,
      description: entry.description,
      message: entry.message,
      definition: entry,
      passed: failures.length === 0,
      duration_ms: Math.round(performance.now() - started),
      failures,
      failure_stage: trace.events.find((event) => event.status === 'FAILED')?.stage ?? null,
      trace: trace.events,
      draft,
      pricing,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (activeFinish) {
      activeFinish('FAILED', message);
    } else {
      trace.observe('runner', 'RFQ evaluation runner', 'FAILED', message);
    }
    return {
      id: entry.id,
      description: entry.description,
      message: entry.message,
      definition: entry,
      passed: false,
      duration_ms: Math.round(performance.now() - started),
      failures: [message],
      failure_stage: trace.events.findLast((event) => event.status === 'FAILED')?.stage ?? 'runner',
      trace: trace.events,
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
  const startedAt = new Date();
  const setupTrace = createTraceRecorder({ enabled: cfg.trace });
  if (cfg.trace) console.log(`RFQ debug trace against ${cfg.apiURL}`);
  await waitForAPI(cfg, setupTrace);
  const token = await accessToken(cfg, setupTrace);
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
    setup_trace: setupTrace.events,
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
