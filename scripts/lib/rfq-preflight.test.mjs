import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { buildTypePreflight } from './rfq-preflight.mjs';

const ROOT = process.cwd();
const BASE = {
  go_ready: true,
  pnpm_ready: true,
  api_ready: true,
  database_url_ready: true,
  admin_database_url_ready: true,
  database_ready: true,
  admin_database_ready: true,
  pgvector_ready: true,
  llm_provider_ready: true,
  embeddings_provider_ready: true,
  anthropic_ready: true,
  openai_ready: true,
};

describe('rfq-preflight.mjs', () => {
  it('blocks the integration surface when its database is unavailable', () => {
    const preflight = buildTypePreflight(
      {
        id: 'pipeline_integration',
        source_files: ['apps/api/internal/integration/rfq_pipeline_test.go'],
        requires_database: true,
        requires_api: false,
        uses_ai: false,
      },
      { ...BASE, database_ready: false },
      ROOT,
    );

    assert.equal(preflight.ready, false);
    assert.equal(preflight.checks.find((check) => check.id === 'database').status, 'BLOCKED');
  });

  it('requires both AI providers without exposing either key', () => {
    const preflight = buildTypePreflight(
      {
        id: 'live_suite',
        source_files: ['scripts/rfq-eval.mjs'],
        requires_database: true,
        requires_api: true,
        uses_ai: true,
      },
      { ...BASE, openai_ready: false },
      ROOT,
    );

    assert.equal(preflight.ready, false);
    assert.deepEqual(
      preflight.checks.filter((check) => ['anthropic', 'openai'].includes(check.id)),
      [
        {
          id: 'anthropic',
          label: 'Anthropic configurado',
          status: 'READY',
          detail: 'Se comprueba la presencia de la clave sin exponer su valor.',
        },
        {
          id: 'openai',
          label: 'OpenAI configurado',
          status: 'BLOCKED',
          detail: 'Se comprueba la presencia de la clave sin exponer su valor.',
        },
      ],
    );
  });

  it('marks a deterministic unit surface ready with only Go and source code', () => {
    const preflight = buildTypePreflight(
      {
        id: 'extractor_unit',
        source_files: ['apps/api/internal/ai/rfq_extractor_test.go'],
        requires_database: false,
        requires_api: false,
        uses_ai: false,
      },
      BASE,
      ROOT,
    );

    assert.equal(preflight.ready, true);
    assert.deepEqual(
      preflight.checks.map((check) => check.id),
      ['go', 'sources'],
    );
  });
});
