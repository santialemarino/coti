import { spawn } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import path from 'node:path';

import { packageManagerInvocation } from './process-command.mjs';

const CACHE_MILLISECONDS = 3_000;

export function createPreflightManager({
  root,
  types,
  apiURL = process.env.RFQ_EVAL_API_URL ?? 'http://127.0.0.1:8001',
  probes = {},
}) {
  let cached = null;
  let cachedAt = 0;
  const commandProbe = probes.command ?? probeCommand;
  const httpProbe = probes.http ?? probeHTTP;
  const tcpProbe = probes.tcp ?? probeTCP;

  async function all({ force = false } = {}) {
    if (!force && cached && Date.now() - cachedAt < CACHE_MILLISECONDS) return cached;

    const localEnv = { ...process.env, ...readEnvFile(path.join(root, 'apps', 'api', '.env')) };
    const databaseURL = localEnv.TEST_DATABASE_URL ?? localEnv.DATABASE_URL ?? '';
    const adminDatabaseURL = localEnv.TEST_DATABASE_ADMIN_URL ?? localEnv.DATABASE_ADMIN_URL ?? '';
    const packageManager = packageManagerInvocation(['--version']);
    const [goReady, pnpmReady, apiReady, databaseReady, adminDatabaseReady] = await Promise.all([
      commandProbe('go', ['version']),
      commandProbe(packageManager.command, packageManager.args),
      httpProbe(`${apiURL.replace(/\/$/, '')}/health`),
      databaseURL ? tcpProbe(databaseURL) : Promise.resolve(false),
      adminDatabaseURL ? tcpProbe(adminDatabaseURL) : Promise.resolve(false),
    ]);
    const availability = {
      go_ready: goReady,
      pnpm_ready: pnpmReady,
      api_ready: apiReady,
      database_url_ready: Boolean(databaseURL),
      admin_database_url_ready: Boolean(adminDatabaseURL),
      database_ready: databaseReady,
      admin_database_ready: adminDatabaseReady,
      pgvector_ready: hasPgvectorMigration(root),
      llm_provider_ready: localEnv.AI_LLM_PROVIDER === 'anthropic',
      embeddings_provider_ready: localEnv.AI_EMBEDDINGS_PROVIDER === 'openai',
      anthropic_ready: Boolean(localEnv.AI_ANTHROPIC_API_KEY),
      openai_ready: Boolean(localEnv.AI_OPENAI_API_KEY),
    };

    cached = Object.fromEntries(
      types.map((type) => [type.id, buildTypePreflight(type, availability, root)]),
    );
    cachedAt = Date.now();
    return cached;
  }

  return {
    all,
    async forType(typeID, options = {}) {
      return (await all(options))[typeID] ?? null;
    },
  };
}

export function buildTypePreflight(type, availability, root) {
  const checks = [
    readiness(
      'sources',
      'Código fuente disponible',
      type.source_files.every((file) => fs.existsSync(path.resolve(root, file))),
      'Los archivos relacionados deben existir en esta rama.',
    ),
  ];

  if (!type.uses_ai) {
    checks.unshift(
      readiness('go', 'Go disponible', availability.go_ready, 'Necesario para ejecutar los tests.'),
    );
  }

  if (type.id === 'pipeline_integration') {
    checks.push(
      readiness(
        'pnpm',
        'Comando canónico disponible',
        availability.pnpm_ready,
        'El Lab ejecuta pnpm test:rfq:integration.',
      ),
      readiness(
        'database_urls',
        'Variables de base de test',
        availability.database_url_ready && availability.admin_database_url_ready,
        'Requiere las URLs restringida y administradora.',
      ),
      readiness(
        'admin_database',
        'Rol administrador accesible',
        availability.admin_database_ready,
        'La suite usa este rol para preparar y aislar cada escenario.',
      ),
    );
  }

  if (type.requires_database) {
    checks.push(
      readiness(
        'database',
        'PostgreSQL accesible',
        availability.database_ready,
        availability.database_url_ready
          ? 'Se verificó la conexión TCP configurada.'
          : 'No hay una URL de base configurada.',
      ),
      readiness(
        'pgvector',
        'Migración de pgvector presente',
        availability.pgvector_ready,
        'La extensión se valida nuevamente dentro de la suite de integración.',
      ),
    );
  }

  if (type.requires_api) {
    checks.push(
      readiness(
        'api',
        'API RFQ disponible',
        availability.api_ready,
        'El endpoint /health debe responder antes de una evaluación live.',
      ),
    );
  }

  if (type.uses_ai) {
    checks.push(
      readiness(
        'llm_provider',
        'Extractor configurado con Anthropic',
        availability.llm_provider_ready,
        'AI_LLM_PROVIDER debe estar configurado como anthropic.',
      ),
      readiness(
        'anthropic',
        'Anthropic configurado',
        availability.anthropic_ready,
        'Se comprueba la presencia de la clave sin exponer su valor.',
      ),
      readiness(
        'embeddings_provider',
        'Embeddings configurados con OpenAI',
        availability.embeddings_provider_ready,
        'AI_EMBEDDINGS_PROVIDER debe estar configurado como openai.',
      ),
      readiness(
        'openai',
        'OpenAI configurado',
        availability.openai_ready,
        'Se comprueba la presencia de la clave sin exponer su valor.',
      ),
    );
  }

  return {
    ready: checks.every((check) => check.status === 'READY'),
    checked_at: new Date().toISOString(),
    checks,
  };
}

function readiness(id, label, ready, detail) {
  return { id, label, status: ready ? 'READY' : 'BLOCKED', detail };
}

function hasPgvectorMigration(root) {
  const directory = path.join(root, 'apps', 'api', 'migrations');
  if (!fs.existsSync(directory)) return false;
  return fs.readdirSync(directory).some((name) => {
    if (!name.endsWith('.sql')) return false;
    return /CREATE\s+EXTENSION(?:\s+IF\s+NOT\s+EXISTS)?\s+vector/i.test(
      fs.readFileSync(path.join(directory, name), 'utf8'),
    );
  });
}

function probeCommand(command, args) {
  return new Promise((resolve) => {
    let settled = false;
    const child = spawn(command, args, { shell: false, stdio: 'ignore', windowsHide: true });
    const finish = (ready) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(ready);
    };
    const timer = setTimeout(() => {
      child.kill();
      finish(false);
    }, 2_000);
    child.once('error', () => finish(false));
    child.once('close', (code) => finish(code === 0));
  });
}

async function probeHTTP(url) {
  try {
    const response = await fetch(url, { signal: AbortSignal.timeout(800) });
    return response.ok;
  } catch {
    return false;
  }
}

function probeTCP(connectionString) {
  return new Promise((resolve) => {
    let parsed;
    try {
      parsed = new URL(connectionString);
    } catch {
      resolve(false);
      return;
    }
    const socket = net.createConnection({
      host: parsed.hostname === 'localhost' ? '127.0.0.1' : parsed.hostname,
      port: Number(parsed.port || 5432),
    });
    let settled = false;
    const finish = (ready) => {
      if (settled) return;
      settled = true;
      socket.destroy();
      resolve(ready);
    };
    socket.setTimeout(800, () => finish(false));
    socket.once('connect', () => finish(true));
    socket.once('error', () => finish(false));
  });
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
