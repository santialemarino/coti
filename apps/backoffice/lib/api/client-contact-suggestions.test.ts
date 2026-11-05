import { expect, it } from 'vitest';

import { isMergeable, mapContactSuggestion } from '@/lib/api/client-contact-suggestions';

const raw = {
  id: 'case',
  source_client_id: 'source',
  candidate_client_id: 'target',
  candidate_name: 'Customer',
  source_name: 'Loser',
  source_registered: false,
  candidate_registered: true,
  tax_id: '20123456786',
  kind: 'EMAIL' as const,
  value: 'one@example.test',
  declared_name: 'Declared',
  original_text: 'Original',
  created_at: 'date',
};

it('maps every retained declaration and both profiles of the case', () => {
  expect(mapContactSuggestion(raw)).toEqual({
    id: 'case',
    sourceClientId: 'source',
    candidateClientId: 'target',
    candidateName: 'Customer',
    sourceName: 'Loser',
    sourceRegistered: false,
    candidateRegistered: true,
    taxId: '20123456786',
    kind: 'EMAIL',
    value: 'one@example.test',
    declaredName: 'Declared',
    originalText: 'Original',
    createdAt: 'date',
  });
});

it('offers a merge only between two profiles without a CUIT', () => {
  expect(isMergeable(mapContactSuggestion(raw))).toBe(false);
  expect(isMergeable(mapContactSuggestion({ ...raw, candidate_registered: false }))).toBe(true);
  expect(
    isMergeable(
      mapContactSuggestion({ ...raw, candidate_registered: false, source_client_id: null }),
    ),
  ).toBe(false);
});
