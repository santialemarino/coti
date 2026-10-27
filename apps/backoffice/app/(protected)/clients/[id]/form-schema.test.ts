import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import {
  clientFiscalSchema,
  type ClientFiscalValues,
} from '@/app/(protected)/clients/[id]/form-schema';

const VALUES: ClientFiscalValues = { legalName: '', dni: '', ivaCondition: '' };

function dniIssues(dni: string) {
  const result = clientFiscalSchema(schemaText(true)).safeParse({ ...VALUES, dni });
  return result.success ? [] : result.error.issues.map((issue) => issue.message);
}

describe('clientFiscalSchema DNI', () => {
  it('accepts nothing and a DNI of seven or eight digits', () => {
    expect(dniIssues('')).toEqual([]);
    expect(dniIssues('1.234.567')).toEqual([]);
    expect(dniIssues('28.123.456')).toEqual([]);
  });

  // The CUIT comes only from the customer's account, so the field takes none.
  it('refuses a CUIT and any other length', () => {
    expect(dniIssues('20-12345678-6')).toEqual(['field:dni.invalid']);
    expect(dniIssues('123456')).toEqual(['field:dni.invalid']);
    expect(dniIssues('123456789')).toEqual(['field:dni.invalid']);
  });

  it('settles a DNI to its digits', () => {
    expect(clientFiscalSchema().parse({ ...VALUES, dni: '12.345.678' }).dni).toBe('12345678');
  });
});
