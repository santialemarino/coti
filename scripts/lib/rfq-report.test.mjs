import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';

import { renderRFQDashboard, writeRFQDashboard } from './rfq-report.mjs';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..');

const report = {
  started_at: '2026-08-21T17:20:47.188Z',
  api_url: 'http://127.0.0.1:8001',
  cases_file: 'scripts/fixtures/rfq-eval-cases.json',
  summary: { total: 1, passed: 1, failed: 0 },
  results: [
    {
      id: 'explicit-quantity',
      description: 'Cantidad explicita',
      message: '10 bolsas de cemento',
      passed: true,
      duration_ms: 1200,
      failures: [],
      draft: {
        status: 201,
        body: {
          rfq: { status: 'GENERATED' },
          quote: { current_status: 'DRAFT' },
          items: [{ requested_description: 'cemento', quantity: '10.00' }],
        },
      },
      pricing: null,
    },
  ],
};
const definitions = [
  {
    id: 'explicit-quantity',
    description: 'Cantidad explicita',
    message: '10 bolsas de cemento',
    expected: { item_count: 1 },
  },
];

describe('rfq-report.mjs', () => {
  it('embeds the report safely and provides every detail view', () => {
    const html = renderRFQDashboard(report, definitions);

    assert.match(html, /Reporte QA del motor RFQ/);
    assert.match(html, /Qué se valida/);
    assert.match(html, /Respuesta/);
    assert.match(html, /Código del caso/);
    assert.doesNotMatch(html, /10 bolsas de cemento/);
    const encoded = html.match(/atob\('([^']+)'\)/)?.[1];
    assert.ok(encoded);
    const payload = JSON.parse(Buffer.from(encoded, 'base64').toString('utf8'));
    assert.deepEqual(payload.results[0].definition.expected, { item_count: 1 });
  });

  it('writes a standalone HTML artifact', () => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'coti-rfq-report-'));
    const output = path.join(directory, 'report.html');

    assert.equal(writeRFQDashboard(report, definitions, output), output);
    assert.equal(fs.readFileSync(output, 'utf8').startsWith('<!doctype html>'), true);
  });

  it('runs the real report command with explicit files', () => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'coti-rfq-report-cli-'));
    const input = path.join(directory, 'report.json');
    const cases = path.join(directory, 'cases.json');
    const output = path.join(directory, 'dashboard.html');
    fs.writeFileSync(input, JSON.stringify(report));
    fs.writeFileSync(cases, JSON.stringify(definitions));

    const result = spawnSync(
      process.execPath,
      ['scripts/rfq-report.mjs', '--input', input, '--cases', cases, '--output', output],
      { cwd: ROOT, encoding: 'utf8' },
    );

    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /Dashboard:/);
    assert.equal(fs.existsSync(output), true);
  });
});
