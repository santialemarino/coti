import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, it } from 'node:test';

import { createGoTestCollector, parseGoTestEvent } from './go-test-events.mjs';

describe('go-test-events.mjs', () => {
  it('turns Go JSON events into a source-linked test result', () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'coti-go-events-'));
    const source = path.join(root, 'sample_test.go');
    fs.writeFileSync(
      source,
      [
        'package sample',
        '',
        'func TestCatalogMatch_FlagsNoMatch(t *testing.T) {',
        '\tt.Log("decision: NO_MATCH")',
        '}',
      ].join('\n'),
    );
    const collector = createGoTestCollector({ root, sourceFiles: ['sample_test.go'] });

    collector.accept(
      JSON.stringify({
        Action: 'run',
        Package: 'example/sample',
        Test: 'TestCatalogMatch_FlagsNoMatch',
      }),
    );
    collector.accept(
      JSON.stringify({
        Action: 'output',
        Package: 'example/sample',
        Test: 'TestCatalogMatch_FlagsNoMatch',
        Output: 'decision: NO_MATCH\n',
      }),
    );
    collector.accept(
      JSON.stringify({
        Action: 'pass',
        Package: 'example/sample',
        Test: 'TestCatalogMatch_FlagsNoMatch',
        Elapsed: 0.012,
      }),
    );

    assert.deepEqual(collector.summary(), {
      total: 1,
      passed: 1,
      failed: 0,
      skipped: 0,
      running: 0,
    });
    assert.deepEqual(collector.snapshot()[0], {
      id: 'example/sample:TestCatalogMatch_FlagsNoMatch',
      name: 'TestCatalogMatch_FlagsNoMatch',
      description: 'Catalog Match - Flags No Match',
      package: 'example/sample',
      status: 'PASSED',
      duration_ms: 12,
      output: ['decision: NO_MATCH'],
      error: null,
      source: {
        path: 'sample_test.go',
        line: 3,
        code: 'func TestCatalogMatch_FlagsNoMatch(t *testing.T) {\n\tt.Log("decision: NO_MATCH")\n}',
      },
    });
  });

  it('keeps non-JSON process output outside the structured collector', () => {
    assert.equal(parseGoTestEvent('> pnpm test:rfq:integration'), null);
    assert.equal(parseGoTestEvent('{bad json'), null);
  });

  it('surfaces package failures that happen before a test starts', () => {
    const collector = createGoTestCollector({ root: process.cwd() });
    collector.accept(
      JSON.stringify({
        Action: 'fail',
        Package: 'example/broken',
        Output: 'build failed\n',
        Elapsed: 0.2,
      }),
    );

    assert.equal(collector.snapshot()[0].status, 'FAILED');
    assert.deepEqual(collector.snapshot()[0].output, ['build failed']);
    assert.equal(collector.snapshot()[0].error, 'build failed');
  });
});
