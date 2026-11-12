import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import { comboSchema, type ComboValues } from '@/app/(protected)/settings/combos/form-schema';

function valid(overrides: Partial<ComboValues> = {}): ComboValues {
  return {
    name: 'Obra gruesa',
    description: '',
    isActive: true,
    items: [
      { productId: 'p1', label: 'Cemento', quantity: '10' },
      { productId: 'p2', label: 'Arena', quantity: '2' },
    ],
    branchIds: ['b1'],
    ...overrides,
  };
}

function issues(values: ComboValues): Record<string, string> {
  const result = comboSchema(schemaText(true)).safeParse(values);
  if (result.success) return {};
  return Object.fromEntries(
    result.error.issues.map((issue) => [issue.path.join('.'), issue.message]),
  );
}

describe('comboSchema', () => {
  it('accepts two products sold in one branch', () => {
    expect(issues(valid())).toEqual({});
  });

  it('needs two distinct products, each picked and with a quantity', () => {
    expect(issues(valid({ items: [{ productId: 'p1', label: '', quantity: '1' }] }))).toMatchObject(
      {
        items: 'field:items.tooFew',
      },
    );
    expect(
      issues(
        valid({
          items: [
            { productId: 'p1', label: '', quantity: '1' },
            { productId: 'p1', label: '', quantity: '0' },
            { productId: '', label: '', quantity: '1' },
          ],
        }),
      ),
    ).toMatchObject({
      'items.1.productId': 'field:items.repeated',
      'items.1.quantity': 'field:items.quantityRequired',
      'items.2.productId': 'field:items.productRequired',
    });
  });

  it('needs a name and a branch', () => {
    expect(issues(valid({ name: ' ', branchIds: [] }))).toMatchObject({
      name: 'field:name.required',
      branchIds: 'field:branches.required',
    });
  });
});
