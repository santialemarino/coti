import { createTranslator } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import {
  grossIncomeRegistrationOf,
  invoicingSettingsSchema,
  type InvoicingSettingsValues,
} from '@/app/(protected)/settings/invoicing/form-schema';
import messages from '@/translations/es.json';

const VALID: InvoicingSettingsValues = {
  ivaCondition: 'REGISTERED',
  pricesIncludeVat: false,
  address: 'Av. Rivadavia 1234',
  grossIncomeKind: 'number',
  grossIncomeNumber: '901-123456-7',
  activityStartedOn: '2020-01-02',
  branches: [],
};

function issues(overrides: Partial<InvoicingSettingsValues>) {
  const result = invoicingSettingsSchema(schemaText(true)).safeParse({ ...VALID, ...overrides });
  return result.success ? [] : result.error.issues.map((issue) => issue.message);
}

function pointOfSaleIssues(...points: string[]) {
  return issues({
    branches: points.map((pointOfSale, i) => ({ branchId: `branch-${i}`, pointOfSale })),
  });
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

describe('invoicingSettingsSchema gross income', () => {
  it('normalizes a number to digits and hyphens', () => {
    const parsed = invoicingSettingsSchema().parse({
      ...VALID,
      grossIncomeNumber: ' 901.123 456-7 ',
    });
    expect(grossIncomeRegistrationOf(parsed)).toBe('901123456-7');
  });

  it('stores an exempt issuer canonically, whatever the number field holds', () => {
    const parsed = invoicingSettingsSchema().parse({
      ...VALID,
      grossIncomeKind: 'exempt',
      grossIncomeNumber: 'leftover',
    });
    expect(grossIncomeRegistrationOf(parsed)).toBe('EXENTO');
  });

  it('tells a missing number from a malformed one', () => {
    expect(issues({ grossIncomeNumber: '' })).toEqual(['field:profile.grossIncomeNumberRequired']);
    for (const raw of ['12-3', 'IIBB 1234567', '1234567890123456', '-123456']) {
      expect(issues({ grossIncomeNumber: raw })).toEqual([
        'field:profile.grossIncomeNumberInvalid',
      ]);
    }
  });

  it('requires the IVA condition', () => {
    expect(issues({ ivaCondition: '' })).toEqual(['field:ivaCondition.required']);
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
      ...VALID,
      ivaCondition: '',
      address: '',
      grossIncomeNumber: '',
      activityStartedOn: '',
    });
    expect(result.success).toBe(false);
    if (result.success) throw new Error('empty profile accepted');
    expect(result.error.issues.map((issue) => issue.message)).toEqual([
      messages.invoicing.settings.profile.addressRequired,
      messages.invoicing.settings.profile.activityStartedOnRequired,
      messages.invoicing.settings.ivaCondition.required,
      messages.invoicing.settings.profile.grossIncomeNumberRequired,
    ]);
  });
});
