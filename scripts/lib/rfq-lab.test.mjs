import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { PassThrough } from 'node:stream';
import { describe, it } from 'node:test';

import {
  buildRunSpec,
  createRunManager,
  listReports,
  loadCustomCases,
  RFQ_TEST_TYPES,
  saveCustomCase,
  validateCustomCase,
} from './rfq-lab.mjs';

describe('rfq-lab.mjs', () => {
  it('publishes AI and deterministic test surfaces explicitly', () => {
    assert.equal(
      RFQ_TEST_TYPES.some((entry) => entry.uses_ai),
      true,
    );
    assert.equal(
      RFQ_TEST_TYPES.some((entry) => !entry.uses_ai),
      true,
    );
    assert.equal(
      RFQ_TEST_TYPES.filter((entry) => entry.uses_ai).every((entry) => entry.providers.length > 0),
      true,
    );
  });

  it('runs the full integration contract through the canonical package script', () => {
    const type = RFQ_TEST_TYPES.find((entry) => entry.id === 'pipeline_integration');
    const spec = buildRunSpec(type, null, {
      root: process.cwd(),
      directory: path.join(process.cwd(), '.artifacts', 'rfq-eval'),
      runID: 'run-1',
      localEnv: {},
    });

    assert.equal(spec.args.at(-1), 'test:rfq:integration');
    assert.match(spec.env.GOFLAGS, /(?:^|\s)-json(?:\s|$)/);
    assert.equal(spec.output_format, 'go-test-json');
  });

  it('keeps structured results while a Go test run is being streamed', async () => {
    let child;
    const spawnProcess = () => {
      child = new EventEmitter();
      child.stdout = new PassThrough();
      child.stderr = new PassThrough();
      return child;
    };
    const manager = createRunManager({
      root: process.cwd(),
      directory: fs.mkdtempSync(path.join(os.tmpdir(), 'coti-rfq-runs-')),
      spawnProcess,
    });
    const run = manager.start('extractor_unit');
    const event = (value) => child.stdout.write(`${JSON.stringify(value)}\n`);

    event({ Action: 'run', Package: 'example/ai', Test: 'TestRFQExtractor' });
    event({
      Action: 'output',
      Package: 'example/ai',
      Test: 'TestRFQExtractor',
      Output: 'validated schema\n',
    });
    event({
      Action: 'pass',
      Package: 'example/ai',
      Test: 'TestRFQExtractor',
      Elapsed: 0.01,
    });
    child.stdout.end();
    child.emit('close', 0);
    await new Promise((resolve) => setImmediate(resolve));

    assert.equal(run.status, 'PASSED');
    assert.deepEqual(run.summary, {
      total: 1,
      passed: 1,
      failed: 0,
      skipped: 0,
      running: 0,
    });
    assert.equal(run.tests[0].status, 'PASSED');
    assert.deepEqual(run.tests[0].output, ['validated schema']);
  });

  it('builds observable expectations for a custom WhatsApp case', () => {
    const entry = validateCustomCase({
      name: 'Pedido cemento',
      message: 'Necesito 10 bolsas de cemento',
      item_count: 1,
      first_description_contains: 'cemento',
      first_quantity: '10',
      first_match_status: 'MATCHED',
    });

    assert.match(entry.id, /^custom-pedido-cemento-/);
    assert.deepEqual(entry.expected.items, [
      {
        description_contains: 'cemento',
        quantity: '10.00',
        match_status: 'MATCHED',
        rationale_present: true,
      },
    ]);
  });

  it('refuses invalid custom expectations', () => {
    assert.throws(
      () => validateCustomCase({ name: 'Bad', message: 'abc', item_count: 51 }),
      /item_count/,
    );
    assert.throws(
      () =>
        validateCustomCase({
          name: 'Bad quantity',
          message: 'cemento',
          first_quantity: 'ten',
        }),
      /first_quantity/,
    );
  });

  it('persists custom cases and indexes valid reports', () => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'coti-rfq-lab-'));
    const saved = saveCustomCase(directory, {
      name: 'Arena',
      message: 'Necesito 2 m3 de arena',
    });
    fs.writeFileSync(
      path.join(directory, 'run.json'),
      JSON.stringify({
        started_at: '2026-08-21T20:00:00.000Z',
        summary: { total: 1, passed: 1, failed: 0 },
        results: [{}],
      }),
    );

    assert.equal(loadCustomCases(directory)[0].id, saved.id);
    assert.deepEqual(listReports(directory)[0].summary, { total: 1, passed: 1, failed: 0 });
  });
});
