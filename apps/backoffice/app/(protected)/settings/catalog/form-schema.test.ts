import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import { productSchema, type ProductValues } from '@/app/(protected)/settings/catalog/form-schema';

const VALUES: ProductValues = {
  code: '',
  name: 'Cemento Portland',
  description: '',
  unit: 'bolsa',
  familyId: '11111111-1111-4111-8111-111111111111',
  subgroupId: '',
  isActive: true,
  price: '',
  minPrice: '',
};

function issuesOf(values: ProductValues) {
  const result = productSchema(schemaText(true)).safeParse(values);
  return result.success
    ? []
    : result.error.issues.map((issue) => ({ path: issue.path.join('.'), message: issue.message }));
}

describe('productSchema prices', () => {
  it('accepts a product with no price, a price alone, and a floor at the price', () => {
    expect(issuesOf(VALUES)).toEqual([]);
    expect(issuesOf({ ...VALUES, price: '12500.50' })).toEqual([]);
    expect(issuesOf({ ...VALUES, price: '100', minPrice: '100' })).toEqual([]);
  });

  it('puts a floor without a price on the price field', () => {
    expect(issuesOf({ ...VALUES, minPrice: '100' })).toEqual([
      { path: 'price', message: 'field:price.requiredWithMin' },
    ]);
  });

  it('puts a floor above the price on the floor field', () => {
    expect(issuesOf({ ...VALUES, price: '100', minPrice: '100.01' })).toEqual([
      { path: 'minPrice', message: 'field:minPrice.abovePrice' },
    ]);
  });

  it('refuses an amount NUMERIC(14,2) cannot hold', () => {
    expect(issuesOf({ ...VALUES, price: '1000000000000' })).toEqual([
      { path: 'price', message: 'shared:amountTooHigh' },
    ]);
  });
});
