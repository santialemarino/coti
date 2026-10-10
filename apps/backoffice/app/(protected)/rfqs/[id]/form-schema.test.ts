import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import {
  clientFiscalSchema,
  type ClientFiscalValues,
} from '@/app/(protected)/rfqs/[id]/form-schema';

const VALUES: ClientFiscalValues = { legalName: '', taxId: '', ivaCondition: '' };

function taxIdIssues(taxId: string) {
  const result = clientFiscalSchema(schemaText(true)).safeParse({ ...VALUES, taxId });
  return result.success ? [] : result.error.issues.map((issue) => issue.message);
}

describe('clientFiscalSchema tax id', () => {
  it('accepts nothing, a CUIT with a valid check digit and a DNI', () => {
    expect(taxIdIssues('')).toEqual([]);
    expect(taxIdIssues('20-12345678-6')).toEqual([]);
    expect(taxIdIssues('30701234568')).toEqual([]);
    expect(taxIdIssues('28.123.456')).toEqual([]);
  });

  // The rule shown is the rule the API enforces, so a wrong last digit never makes the round trip.
  it('refuses a CUIT whose check digit does not match', () => {
    expect(taxIdIssues('20-12345678-5')).toEqual(['field:taxId.checkDigit']);
  });

  it('refuses a number that is neither a CUIT nor a DNI', () => {
    expect(taxIdIssues('123456')).toEqual(['field:taxId.invalid']);
  });
});
