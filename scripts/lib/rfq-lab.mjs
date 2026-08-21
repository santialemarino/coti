import { spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

const CUSTOM_CASES_FILE = 'custom-cases.json';
const RUN_LOG_LIMIT = 2_000;

export const RFQ_TEST_TYPES = [
  {
    id: 'live_custom',
    label: 'Mensaje WhatsApp personalizado',
    component: 'Flujo completo',
    description: 'Envía un caso guardado por el endpoint mock y valida la cotización resultante.',
    uses_ai: true,
    providers: ['Anthropic', 'OpenAI'],
    requires_api: true,
    requires_database: true,
    accepts_case: true,
    source_files: [
      'scripts/rfq-eval.mjs',
      'apps/api/internal/services/rfq_service.go',
      'apps/api/internal/ai/rfq_extractor.go',
    ],
  },
  {
    id: 'live_suite',
    label: 'Suite WhatsApp completa',
    component: 'Flujo completo',
    description: 'Ejecuta todos los escenarios versionados contra los proveedores reales.',
    uses_ai: true,
    providers: ['Anthropic', 'OpenAI'],
    requires_api: true,
    requires_database: true,
    accepts_case: false,
    source_files: ['scripts/fixtures/rfq-eval-cases.json', 'scripts/rfq-eval.mjs'],
  },
  {
    id: 'extractor_unit',
    label: 'Extractor RFQ',
    component: 'Extracción',
    description: 'Valida prompt, schema forzado y mapeo con respuestas fijas del modelo.',
    uses_ai: false,
    providers: [],
    requires_api: false,
    requires_database: false,
    accepts_case: false,
    source_files: ['apps/api/internal/ai/rfq_extractor_test.go'],
  },
  {
    id: 'service_unit',
    label: 'Servicio RFQ',
    component: 'Orquestación',
    description:
      'Prueba recepción, extracción, matching y armado del draft con dobles controlados.',
    uses_ai: false,
    providers: [],
    requires_api: false,
    requires_database: false,
    accepts_case: false,
    source_files: ['apps/api/internal/services/rfq_service_test.go'],
  },
  {
    id: 'matching_unit',
    label: 'Matching de catálogo',
    component: 'Matching',
    description: 'Prueba umbrales, ambigüedad y NO_MATCH sin pedir embeddings externos.',
    uses_ai: false,
    providers: [],
    requires_api: false,
    requires_database: false,
    accepts_case: false,
    source_files: ['apps/api/internal/services/catalog_match_service_test.go'],
  },
  {
    id: 'pricing_unit',
    label: 'Pricing determinístico',
    component: 'Pricing',
    description: 'Valida aceptación de materiales, precios vigentes, subtotales y total.',
    uses_ai: false,
    providers: [],
    requires_api: false,
    requires_database: false,
    accepts_case: false,
    source_files: ['apps/api/internal/services/quote_service_test.go'],
  },
  {
    id: 'handler_unit',
    label: 'Contrato HTTP',
    component: 'Handler',
    description: 'Valida status HTTP y JSON del endpoint RFQ con un servicio simulado.',
    uses_ai: false,
    providers: [],
    requires_api: false,
    requires_database: false,
    accepts_case: false,
    source_files: ['apps/api/internal/delivery/http/handler/rfq_handler_test.go'],
  },
  {
    id: 'pipeline_integration',
    label: 'Pipeline con PostgreSQL',
    component: 'Integración',
    description: 'Usa PostgreSQL y pgvector reales con extractor y embeddings determinísticos.',
    uses_ai: false,
    providers: [],
    requires_api: false,
    requires_database: true,
    accepts_case: false,
    source_files: ['apps/api/internal/integration/rfq_pipeline_test.go'],
  },
];

export function validateCustomCase(input) {
  if (!input || typeof input !== 'object') throw new Error('The case body must be an object');
  const name = requiredString(input.name, 'name', 80);
  const message = requiredString(input.message, 'message', 4_000);
  const description = optionalString(input.description, 'description', 240) ?? name;
  const itemCount = Number(input.item_count ?? 1);
  if (!Number.isInteger(itemCount) || itemCount < 0 || itemCount > 50) {
    throw new Error('item_count must be an integer between 0 and 50');
  }

  const rfqStatus = enumValue(input.rfq_status ?? 'GENERATED', 'rfq_status', [
    'RECEIVED',
    'GENERATED',
  ]);
  const quoteStatus = enumValue(input.quote_status ?? 'DRAFT', 'quote_status', ['NONE', 'DRAFT']);
  const firstDescription = optionalString(
    input.first_description_contains,
    'first_description_contains',
    160,
  );
  const firstQuantity = optionalDecimal(input.first_quantity, 'first_quantity');
  const firstUnit = optionalString(input.first_unit_contains, 'first_unit_contains', 64);
  const firstMatch = input.first_match_status
    ? enumValue(input.first_match_status, 'first_match_status', [
        'MATCHED',
        'AMBIGUOUS',
        'NO_MATCH',
      ])
    : null;

  const expected = {
    rfq_status: rfqStatus,
    quote_status: quoteStatus === 'NONE' ? null : quoteStatus,
    item_count: itemCount,
    items: [],
  };
  if (itemCount > 0 && (firstDescription || firstQuantity || firstUnit || firstMatch)) {
    expected.items.push({
      ...(firstDescription ? { description_contains: firstDescription } : {}),
      ...(firstQuantity ? { quantity: firstQuantity } : {}),
      ...(firstUnit ? { unit_contains: firstUnit } : {}),
      ...(firstMatch ? { match_status: firstMatch } : {}),
      rationale_present: true,
    });
  }

  return {
    id: `custom-${slug(name)}-${Date.now()}`,
    description,
    message,
    price_after_draft: Boolean(input.price_after_draft),
    expected,
  };
}

export function loadCustomCases(directory) {
  const file = path.join(directory, CUSTOM_CASES_FILE);
  if (!fs.existsSync(file)) return [];
  const value = JSON.parse(fs.readFileSync(file, 'utf8'));
  return Array.isArray(value) ? value : [];
}

export function saveCustomCase(directory, input) {
  const entry = validateCustomCase(input);
  const cases = loadCustomCases(directory);
  cases.push(entry);
  fs.mkdirSync(directory, { recursive: true });
  fs.writeFileSync(path.join(directory, CUSTOM_CASES_FILE), `${JSON.stringify(cases, null, 2)}\n`);
  return entry;
}

export function listReports(directory) {
  if (!fs.existsSync(directory)) return [];
  return fs
    .readdirSync(directory)
    .filter((name) => name.endsWith('.json') && name !== CUSTOM_CASES_FILE)
    .map((name) => {
      const file = path.join(directory, name);
      try {
        const report = JSON.parse(fs.readFileSync(file, 'utf8'));
        if (!report.summary || !Array.isArray(report.results)) return null;
        return {
          name,
          dashboard_url: `/reports/${name.replace(/\.json$/i, '.html')}`,
          started_at: report.started_at,
          summary: report.summary,
        };
      } catch {
        return null;
      }
    })
    .filter(Boolean)
    .sort((left, right) => String(right.started_at).localeCompare(String(left.started_at)))
    .slice(0, 30);
}

export function createRunManager({ root, directory, spawnProcess = spawn }) {
  const runs = new Map();
  const localEnv = readEnvFile(path.join(root, 'apps', 'api', '.env'));

  return {
    get(id) {
      return runs.get(id) ?? null;
    },
    start(typeID, caseID) {
      const type = RFQ_TEST_TYPES.find((entry) => entry.id === typeID);
      if (!type) throw new Error(`Unknown test type: ${typeID}`);
      const customCases = loadCustomCases(directory);
      const selectedCase = caseID ? customCases.find((entry) => entry.id === caseID) : null;
      if (type.accepts_case && !selectedCase) throw new Error('A saved custom case is required');

      const id = randomUUID();
      const spec = commandFor(type, selectedCase, { root, directory, runID: id, localEnv });
      const run = {
        id,
        type_id: type.id,
        type_label: type.label,
        case_id: selectedCase?.id ?? null,
        status: 'RUNNING',
        exit_code: null,
        started_at: new Date().toISOString(),
        finished_at: null,
        logs: [],
        report_url: null,
      };
      runs.set(id, run);

      const child = spawnProcess(spec.command, spec.args, {
        cwd: spec.cwd,
        env: spec.env,
        shell: false,
        windowsHide: true,
      });
      appendOutput(child.stdout, 'stdout', run);
      appendOutput(child.stderr, 'stderr', run);
      child.on('error', (error) => {
        pushLog(run, 'stderr', error.message);
      });
      child.on('close', (code) => {
        run.exit_code = code ?? 1;
        run.status = code === 0 ? 'PASSED' : 'FAILED';
        run.finished_at = new Date().toISOString();
        if (spec.reportPath && fs.existsSync(spec.reportPath.replace(/\.json$/i, '.html'))) {
          run.report_url = `/reports/${path.basename(spec.reportPath).replace(/\.json$/i, '.html')}`;
        }
      });
      return run;
    },
  };
}

function commandFor(type, selectedCase, { root, directory, runID, localEnv }) {
  const apiDir = path.join(root, 'apps', 'api');
  const env = {
    ...process.env,
    ...localEnv,
    TEST_DATABASE_URL: ipv4DatabaseURL(localEnv.DATABASE_URL),
    TEST_DATABASE_ADMIN_URL: ipv4DatabaseURL(localEnv.DATABASE_ADMIN_URL),
  };
  const goTest = (args) => ({
    command: 'go',
    args: ['test', '-v', '-count=1', ...args],
    cwd: apiDir,
    env,
  });

  switch (type.id) {
    case 'extractor_unit':
      return goTest(['-run', '^Test(RFQExtractor|RFQExtractionSchema)', './internal/ai']);
    case 'service_unit':
      return goTest(['-run', '^TestRFQService_', './internal/services']);
    case 'matching_unit':
      return goTest(['-run', '^TestCatalogMatchService', './internal/services']);
    case 'pricing_unit':
      return goTest(['-run', '^TestQuoteService_AcceptMaterials', './internal/services']);
    case 'handler_unit':
      return goTest([
        '-run',
        '^Test(RFQHandler|ToTextRFQDraftResponse)',
        './internal/delivery/http/handler',
      ]);
    case 'pipeline_integration':
      return goTest([
        '-tags=integration',
        '-run',
        '^Test(RFQPipeline|RFQTextDraftRoute|WhatsAppMockRoute)',
        './internal/integration',
      ]);
    case 'live_suite':
      return liveCommand(root, directory, runID, null, env);
    case 'live_custom':
      return liveCommand(root, directory, runID, selectedCase, env);
    default:
      throw new Error(`No command is registered for ${type.id}`);
  }
}

function liveCommand(root, directory, runID, selectedCase, env) {
  fs.mkdirSync(directory, { recursive: true });
  const reportPath = path.join(directory, `lab-${runID}.json`);
  const args = [
    path.join(root, 'scripts', 'rfq-eval.mjs'),
    '--trace',
    '--api-url',
    process.env.RFQ_EVAL_API_URL ?? 'http://127.0.0.1:8001',
    '--wait-seconds',
    '5',
    '--report',
    reportPath,
  ];
  if (selectedCase) {
    const casesPath = path.join(directory, `lab-${runID}-case.json`);
    fs.writeFileSync(casesPath, `${JSON.stringify([selectedCase], null, 2)}\n`);
    args.push('--cases', casesPath, '--case', selectedCase.id);
  }
  return { command: process.execPath, args, cwd: root, env, reportPath };
}

function appendOutput(stream, channel, run) {
  if (!stream) return;
  let pending = '';
  stream.setEncoding('utf8');
  stream.on('data', (chunk) => {
    pending += chunk;
    const lines = pending.split(/\r?\n/);
    pending = lines.pop() ?? '';
    for (const line of lines) pushLog(run, channel, line);
  });
  stream.on('end', () => {
    if (pending) pushLog(run, channel, pending);
  });
}

function pushLog(run, channel, text) {
  run.logs.push({ index: run.logs.length, channel, text, at: new Date().toISOString() });
  if (run.logs.length > RUN_LOG_LIMIT) run.logs.splice(0, run.logs.length - RUN_LOG_LIMIT);
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

function ipv4DatabaseURL(value) {
  return typeof value === 'string' ? value.replace('localhost', '127.0.0.1') : value;
}

function requiredString(value, field, max) {
  if (typeof value !== 'string' || value.trim() === '') throw new Error(`${field} is required`);
  if ([...value.trim()].length > max) throw new Error(`${field} cannot exceed ${max} characters`);
  return value.trim();
}

function optionalString(value, field, max) {
  if (value === undefined || value === null || value === '') return null;
  if (typeof value !== 'string') throw new Error(`${field} must be a string`);
  return requiredString(value, field, max);
}

function optionalDecimal(value, field) {
  if (value === undefined || value === null || value === '') return null;
  const raw = String(value).trim();
  if (!/^(?:0|[1-9]\d*)(?:\.\d{1,2})?$/.test(raw)) {
    throw new Error(`${field} must be a non-negative decimal with at most two decimals`);
  }
  return Number(raw).toFixed(2);
}

function enumValue(value, field, allowed) {
  if (!allowed.includes(value)) throw new Error(`${field} must be one of ${allowed.join(', ')}`);
  return value;
}

function slug(value) {
  const normalized = value
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
    .slice(0, 40);
  return normalized || 'case';
}
