import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import { invoicingSettingsSchema } from '@/app/(protected)/settings/invoicing/form-schema';

function pointOfSaleIssues(...points: string[]) {
  const result = invoicingSettingsSchema(schemaText(true)).safeParse({
    ivaCondition: 'REGISTERED',
    pricesIncludeVat: false,
    branches: points.map((pointOfSale, i) => ({ branchId: `branch-${i}`, pointOfSale })),
  });
  return result.success ? [] : result.error.issues.map((issue) => issue.message);
}

describe('invoicingSettingsSchema points of sale', () => {
  it('accepts an empty point of sale and both ends of ARCA’s range', () => {
    expect(pointOfSaleIssues('', '1', '99998')).toEqual([]);
  });

  it('refuses what ARCA cannot number: zero, past the range, or not a whole number', () => {
    for (const raw of ['0', '99999', '1.5', '1e3']) {
      expect(pointOfSaleIssues(raw)).toEqual(['field:pointOfSale.range']);
    }
  });

  it('refuses two branches on one point of sale', () => {
    expect(pointOfSaleIssues('3', '3')).toEqual(['field:pointOfSale.duplicate']);
  });
});
