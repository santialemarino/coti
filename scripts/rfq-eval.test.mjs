import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';

import { evaluateDraft, evaluatePricing, parseArgs, validateCases } from './rfq-eval.mjs';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

describe('rfq-eval.mjs', () => {
  it('accepts a reviewable WhatsApp draft and its priced response', () => {
    const expected = {
      rfq_status: 'GENERATED',
      quote_status: 'DRAFT',
      item_count: 1,
      items: [
        {
          description_contains: 'cemento',
          quantity: '10.00',
          unit_contains: 'bolsa',
          match_status: 'MATCHED',
          rationale_present: true,
        },
      ],
      priced_quote_status: 'QUOTED',
      total_nonzero: true,
      priced_items: [{ pricing_unavailable: false }],
    };
    const item = {
      requested_description: '10 bolsas de cemento',
      quantity: '10.00',
      unit: 'bolsas',
      match_status: 'MATCHED',
      quantity_rationale: 'El cliente pidio 10 bolsas.',
      pricing_unavailable: null,
    };

    assert.deepEqual(
      evaluateDraft(expected, 201, {
        rfq: { status: 'GENERATED' },
        quote: { current_status: 'DRAFT' },
        items: [item],
      }),
      [],
    );
    assert.deepEqual(
      evaluatePricing(expected, 200, {
        quote: { current_status: 'QUOTED' },
        version: { total: '95000.00' },
        items: [{ ...item, pricing_unavailable: false }],
      }),
      [],
    );
  });

  it('reports every observable mismatch', () => {
    const failures = evaluateDraft(
      {
        rfq_status: 'GENERATED',
        quote_status: 'DRAFT',
        item_count: 1,
        items: [{ quantity: '10.00', rationale_present: true }],
      },
      201,
      {
        rfq: { status: 'RECEIVED' },
        quote: null,
        items: [{ quantity: '0.00', quantity_rationale: null }],
      },
    );

    assert.equal(failures.length, 4);
    assert.match(failures.join('\n'), /rfq\.status/);
    assert.match(failures.join('\n'), /quote\.current_status/);
    assert.match(failures.join('\n'), /quantity/);
    assert.match(failures.join('\n'), /quantity_rationale/);
  });

  it('validates case ids and messages before making a request', () => {
    assert.throws(
      () =>
        validateCases([
          { id: 'same', message: 'one', expected: {} },
          { id: 'same', message: 'two', expected: {} },
        ]),
      /Duplicate case id/,
    );
    assert.throws(() => validateCases([{ id: 'empty', message: '', expected: {} }]), /message/);
  });

  it('parses flags and prints help without contacting the API', () => {
    assert.deepEqual(
      parseArgs(['--price', '--verbose', '--branch', 'branch-id', '--case', 'explicit']),
      {
        price: true,
        verbose: true,
        branchID: 'branch-id',
        caseID: 'explicit',
      },
    );

    const result = spawnSync(process.execPath, ['scripts/rfq-eval.mjs', '--help'], {
      cwd: ROOT,
      encoding: 'utf8',
    });
    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /Usage: pnpm eval:rfq/);
  });
});
