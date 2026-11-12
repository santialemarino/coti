import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import {
  promotionSchema,
  type PromotionValues,
} from '@/app/(protected)/settings/promotions/form-schema';
import { EMPTY_PROMOTION } from '@/app/(protected)/settings/promotions/promotion-values';

const PRODUCT = { kind: 'product' as const, targetId: 'p1', label: 'Cemento', minQuantity: '' };
const FAMILY = { kind: 'family' as const, targetId: 'f1', label: '', minQuantity: '' };

function valid(overrides: Partial<PromotionValues> = {}): PromotionValues {
  return {
    ...EMPTY_PROMOTION,
    name: 'Cemento 10 %',
    actionValue: '10',
    conditions: [PRODUCT],
    ...overrides,
  };
}

// Every message the schema raised, keyed by the path it was raised on.
function issues(values: PromotionValues): Record<string, string> {
  const result = promotionSchema(schemaText(true)).safeParse(values);
  if (result.success) return {};
  return Object.fromEntries(
    result.error.issues.map((issue) => [issue.path.join('.'), issue.message]),
  );
}

describe('promotionSchema', () => {
  it('accepts a percentage off one product', () => {
    expect(issues(valid())).toEqual({});
  });

  it('asks for a name', () => {
    expect(issues(valid({ name: '  ' }))).toMatchObject({ name: 'field:name.required' });
  });

  it('refuses a percentage above 100 and a value of zero', () => {
    expect(issues(valid({ actionValue: '100.01' }))).toMatchObject({
      actionValue: 'field:actionValue.percentageMax',
    });
    expect(issues(valid({ actionValue: '0' }))).toMatchObject({
      actionValue: 'field:actionValue.required',
    });
    // A fixed amount has no ceiling.
    expect(issues(valid({ actionType: 'FIXED_AMOUNT', actionValue: '2500' }))).toEqual({});
  });

  it('allows a special price only where the rule targets units', () => {
    expect(
      issues(
        valid({
          conditionType: 'ITEM_SET',
          actionType: 'SPECIAL_PRICE',
          conditions: [PRODUCT, FAMILY],
        }),
      ),
    ).toMatchObject({ actionType: 'field:actionType.specialPriceScope' });
    expect(issues(valid({ actionType: 'SPECIAL_PRICE', actionValue: '900' }))).toEqual({});
  });

  it('needs two targets for an item set and refuses a repeated one', () => {
    expect(issues(valid({ conditionType: 'ITEM_SET' }))).toMatchObject({
      conditions: 'field:conditions.tooFew',
    });
    expect(
      issues(valid({ conditionType: 'ITEM_SET', conditions: [PRODUCT, { ...PRODUCT }] })),
    ).toMatchObject({ 'conditions.1.targetId': 'field:conditions.repeated' });
  });

  it('reports a row with nothing picked on that row', () => {
    expect(issues(valid({ conditions: [{ ...PRODUCT, targetId: '' }] }))).toMatchObject({
      'conditions.0.targetId': 'field:conditions.targetRequired',
    });
  });

  it('ignores targets on a rule over the total', () => {
    expect(
      issues(valid({ conditionType: 'ON_TOTAL', conditions: [{ ...PRODUCT, targetId: '' }] })),
    ).toEqual({});
    expect(issues(valid({ conditionType: 'ON_TOTAL', minTotal: '0' }))).toMatchObject({
      minTotal: 'field:minTotal.positive',
    });
  });

  it('checks the bands of a tiered rule instead of the single value', () => {
    const tiered = (tiers: PromotionValues['tiers']) =>
      issues(valid({ conditionType: 'QUANTITY_TIERED', actionValue: '', tiers }));

    expect(tiered([])).toMatchObject({ tiers: 'field:tiers.required' });
    expect(tiered([{ fromQuantity: '10', toQuantity: '19', value: '5' }])).toEqual({});
    expect(tiered([{ fromQuantity: '10', toQuantity: '5', value: '5' }])).toMatchObject({
      'tiers.0.toQuantity': 'field:tiers.toBelowFrom',
    });
    // Both bounds are inclusive, so a band starting where another ends overlaps it.
    expect(
      tiered([
        { fromQuantity: '10', toQuantity: '19', value: '5' },
        { fromQuantity: '19', toQuantity: '', value: '8' },
      ]),
    ).toMatchObject({ 'tiers.1.fromQuantity': 'field:tiers.overlap' });
    expect(
      tiered([
        { fromQuantity: '10', toQuantity: '19', value: '5' },
        { fromQuantity: '20', toQuantity: '', value: '8' },
      ]),
    ).toEqual({});
  });

  it('allows a one-day window and refuses an end before the start', () => {
    expect(issues(valid({ validFrom: '2026-12-01', validTo: '2026-12-01' }))).toEqual({});
    expect(issues(valid({ validFrom: '2026-12-02', validTo: '2026-12-01' }))).toMatchObject({
      validTo: 'field:validTo.beforeFrom',
    });
  });

  it('takes a whole, non-negative priority', () => {
    expect(issues(valid({ priority: '-1' }))).toMatchObject({
      priority: 'field:priority.required',
    });
    expect(issues(valid({ priority: '' }))).toMatchObject({ priority: 'field:priority.required' });
  });
});
