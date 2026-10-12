import { createTranslator } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import { invoicingSettingsSchema } from '@/app/(protected)/settings/invoicing/form-schema';
import messages from '@/translations/es.json';

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

describe('fiscal profile validation messages', () => {
  it('resolves every validation message from the actual catalog', () => {
    const onError = (error: Error) => {
      throw error;
    };
    const field = createTranslator({
      locale: 'es',
      messages,
      namespace: 'invoicing.settings',
      onError,
    });
    const shared = createTranslator({
      locale: 'es',
      messages,
      namespace: 'common.form.errors',
      onError,
    });
    const schema = invoicingSettingsSchema({
      field: (key, values) => field(key as Parameters<typeof field>[0], values),
      shared: (key, values) => shared(key as Parameters<typeof shared>[0], values),
    });
    const result = schema.safeParse({
      ivaCondition: 'REGISTERED',
      pricesIncludeVat: true,
      branches: [],
      address: '',
      grossIncomeRegistration: '',
      activityStartedOn: '',
    });
    expect(result.success).toBe(false);
    if (result.success) throw new Error('empty profile accepted');
    expect(result.error.issues.map((issue) => issue.message)).toEqual([
      messages.invoicing.settings.profile.addressRequired,
      messages.invoicing.settings.profile.grossIncomeRegistrationRequired,
      messages.invoicing.settings.profile.activityStartedOnRequired,
    ]);
  });
});
