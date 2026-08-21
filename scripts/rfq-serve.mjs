import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

import { renderRFQLab } from './lib/rfq-lab-page.mjs';
import {
  createRunManager,
  listReports,
  loadCustomCases,
  RFQ_TEST_TYPES,
  saveCustomCase,
} from './lib/rfq-lab.mjs';

const ROOT = process.cwd();
const DEFAULT_DIRECTORY = path.join(ROOT, '.artifacts', 'rfq-eval');

export function parseServeArgs(args) {
  const options = {};
  for (let i = 0; i < args.length; i += 1) {
    if (args[i] === '--help' || args[i] === '-h') {
      options.help = true;
      continue;
    }
    const key = { '--directory': 'directory', '--host': 'host', '--port': 'port' }[args[i]];
    if (!key || !args[i + 1]) throw new Error(`Unknown or incomplete argument: ${args[i]}`);
    options[key] = args[i + 1];
    i += 1;
  }
  return options;
}

export function latestDashboard(directory) {
  if (!fs.existsSync(directory)) return null;
  const files = fs
    .readdirSync(directory)
    .filter((name) => name.endsWith('.html'))
    .map((name) => path.join(directory, name))
    .sort((left, right) => fs.statSync(right).mtimeMs - fs.statSync(left).mtimeMs);
  return files[0] ?? null;
}

export function createRFQReportServer(directory, options = {}) {
  const resolvedDirectory = path.resolve(directory);
  const root = path.resolve(options.root ?? ROOT);
  const runManager = options.runManager ?? createRunManager({ root, directory: resolvedDirectory });

  return http.createServer(async (request, response) => {
    const url = new URL(request.url ?? '/', 'http://localhost');
    try {
      if (request.method === 'GET' && url.pathname === '/') {
        sendHTML(response, renderRFQLab());
        return;
      }
      if (request.method === 'GET' && url.pathname === '/latest') {
        sendDashboard(response, latestDashboard(resolvedDirectory));
        return;
      }
      if (request.method === 'GET' && url.pathname === '/api/state') {
        sendJSON(response, 200, {
          types: RFQ_TEST_TYPES,
          cases: loadCustomCases(resolvedDirectory),
          reports: listReports(resolvedDirectory),
          api_online: await apiOnline(),
        });
        return;
      }
      if (request.method === 'POST' && url.pathname === '/api/cases') {
        requireSameOrigin(request);
        const entry = saveCustomCase(resolvedDirectory, await readJSON(request));
        sendJSON(response, 201, { case: entry });
        return;
      }
      if (request.method === 'POST' && url.pathname === '/api/runs') {
        requireSameOrigin(request);
        const body = await readJSON(request);
        const type = RFQ_TEST_TYPES.find((entry) => entry.id === body.type_id);
        if (!type) throw new HTTPError(400, 'Unknown test type');
        if (type.uses_ai && body.confirm_ai !== true) {
          throw new HTTPError(409, 'AI consumption must be confirmed before this run');
        }
        const run = runManager.start(type.id, body.case_id ?? null);
        sendJSON(response, 201, run);
        return;
      }
      if (request.method === 'GET' && url.pathname.startsWith('/api/runs/')) {
        const run = runManager.get(url.pathname.slice('/api/runs/'.length));
        if (!run) throw new HTTPError(404, 'Run not found');
        sendJSON(response, 200, run);
        return;
      }
      if (request.method === 'GET' && url.pathname.startsWith('/reports/')) {
        sendDashboard(
          response,
          reportPath(resolvedDirectory, url.pathname.slice('/reports/'.length)),
        );
        return;
      }
      if (request.method === 'GET' && url.pathname.endsWith('.html')) {
        sendDashboard(response, reportPath(resolvedDirectory, url.pathname.slice(1)));
        return;
      }
      sendJSON(response, 404, { error: 'Not found' });
    } catch (error) {
      const status = error instanceof HTTPError ? error.status : 400;
      sendJSON(response, status, {
        error: error instanceof Error ? error.message : String(error),
      });
    }
  });
}

class HTTPError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

function reportPath(directory, rawName) {
  const name = path.basename(decodeURIComponent(rawName));
  return name.endsWith('.html') ? path.join(directory, name) : null;
}

function sendDashboard(response, target) {
  if (!target || !fs.existsSync(target)) {
    sendJSON(response, 404, { error: 'No RFQ dashboard exists yet' });
    return;
  }
  response.writeHead(200, responseHeaders('text/html; charset=utf-8'));
  fs.createReadStream(target).pipe(response);
}

function sendHTML(response, html) {
  response.writeHead(200, responseHeaders('text/html; charset=utf-8'));
  response.end(html);
}

function sendJSON(response, status, value) {
  if (response.headersSent) return;
  response.writeHead(status, responseHeaders('application/json; charset=utf-8'));
  response.end(`${JSON.stringify(value)}\n`);
}

function responseHeaders(contentType) {
  return {
    'Cache-Control': 'no-store',
    'Content-Type': contentType,
    'X-Content-Type-Options': 'nosniff',
  };
}

async function readJSON(request) {
  let raw = '';
  for await (const chunk of request) {
    raw += chunk;
    if (Buffer.byteLength(raw) > 64 * 1024) throw new HTTPError(413, 'Request body is too large');
  }
  try {
    return JSON.parse(raw || '{}');
  } catch {
    throw new HTTPError(400, 'Request body must be valid JSON');
  }
}

function requireSameOrigin(request) {
  const origin = request.headers.origin;
  if (!origin) return;
  if (origin !== `http://${request.headers.host}`) {
    throw new HTTPError(403, 'Cross-origin writes are not allowed');
  }
}

async function apiOnline() {
  try {
    const baseURL = (process.env.RFQ_EVAL_API_URL ?? 'http://127.0.0.1:8001').replace(/\/$/, '');
    const response = await fetch(`${baseURL}/health`, {
      signal: AbortSignal.timeout(700),
    });
    return response.ok;
  } catch {
    return false;
  }
}

function usage() {
  return `Usage: pnpm serve:rfq [options]

Serves the interactive RFQ QA Lab without Python or external dependencies.

Options:
  --directory <path> Report directory (default: .artifacts/rfq-eval)
  --host <host>      Bind host (default: 127.0.0.1)
  --port <port>      Bind port (default: 4173)
  --help             Show this help`;
}

function main() {
  const options = parseServeArgs(process.argv.slice(2));
  if (options.help) {
    console.log(usage());
    return;
  }
  const directory = path.resolve(options.directory ?? DEFAULT_DIRECTORY);
  const host = options.host ?? '127.0.0.1';
  const port = Number(options.port ?? 4173);
  if (!Number.isInteger(port) || port < 0 || port > 65535) {
    throw new Error('--port must be an integer between 0 and 65535');
  }

  const server = createRFQReportServer(directory);
  server.listen(port, host, () => {
    const address = server.address();
    const boundPort = typeof address === 'object' && address ? address.port : port;
    console.log(`RFQ QA Lab: http://${host}:${boundPort}`);
    console.log(`Reports: ${path.relative(ROOT, directory)}`);
  });
}

const entrypoint = process.argv[1] ? pathToFileURL(path.resolve(process.argv[1])).href : '';
if (import.meta.url === entrypoint) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  }
}
