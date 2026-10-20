import { expect, it } from 'vitest';

import { mapContactSuggestion } from '@/lib/api/client-contact-suggestions';

it('maps every retained declaration and commercial candidate field', () => {
  expect(
    mapContactSuggestion({
      id: 'case',
      source_client_id: 'source',
      candidate_client_id: 'target',
      candidate_name: 'Customer',
      tax_id: '20123456786',
      kind: 'EMAIL',
      value: 'one@example.test',
      declared_name: 'Declared',
      original_text: 'Original',
      created_at: 'date',
    }),
  ).toEqual({
    id: 'case',
    sourceClientId: 'source',
    candidateClientId: 'target',
    candidateName: 'Customer',
    taxId: '20123456786',
    kind: 'EMAIL',
    value: 'one@example.test',
    declaredName: 'Declared',
    originalText: 'Original',
    createdAt: 'date',
  });
});
