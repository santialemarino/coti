import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  createPromotion,
  setPromotionActive,
  updatePromotion,
} from '@/app/(protected)/settings/promotions/actions';
import type { PromotionValues } from '@/app/(protected)/settings/promotions/form-schema';
import { EMPTY_PROMOTION } from '@/app/(protected)/settings/promotions/promotion-values';
import type { Promotion } from '@/lib/api/promotions';

vi.mock('next/cache', () => ({ revalidatePath: vi.fn() }));
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { revalidatePath } = await import('next/cache');
const { apiRequest } = await import('@/lib/api/client');
const { ApiError } = await import('@/lib/api/errors');

const VALUES: PromotionValues = {
  ...EMPTY_PROMOTION,
  name: 'Cemento 10 %',
  actionValue: '10',
  validFrom: '2026-12-01',
  validTo: '2026-12-31',
  priority: '3',
  conditions: [
    { kind: 'product', targetId: 'p1', label: 'Cemento', minQuantity: '' },
    { kind: 'subgroup', targetId: 's1', label: '', minQuantity: '5' },
  ],
};

function bodySent() {
  return vi.mocked(apiRequest).mock.calls[0]?.[0].body as Record<string, unknown>;
}

beforeEach(() => vi.clearAllMocks());

describe('createPromotion', () => {
  it('sends the rule in the API shape and revalidates the list', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ id: 'new-id' });

    await expect(createPromotion(VALUES)).resolves.toEqual({ ok: true, id: 'new-id' });
    expect(vi.mocked(apiRequest).mock.calls[0]?.[0]).toMatchObject({
      path: '/v1/promotions',
      method: 'POST',
    });
    expect(bodySent()).toEqual({
      name: 'Cemento 10 %',
      description: undefined,
      branch_id: undefined,
      condition_type: 'PER_ITEM',
      action_type: 'PERCENTAGE',
      action_value: '10',
      min_total: undefined,
      // Whole Argentine days: the first second of the start, the last second of the end.
      valid_from: '2026-12-01T00:00:00-03:00',
      valid_to: '2026-12-31T23:59:59-03:00',
      is_active: true,
      is_exclusive: false,
      priority: 3,
      conditions: [
        { product_id: 'p1', family_id: undefined, subgroup_id: undefined, min_quantity: undefined },
        { product_id: undefined, family_id: undefined, subgroup_id: 's1', min_quantity: '5' },
      ],
      tiers: [],
    });
    expect(revalidatePath).toHaveBeenCalledWith('/settings/promotions', 'layout');
  });

  it('drops the single value and the targets the type does not take', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ id: 'x' });

    await createPromotion({
      ...VALUES,
      conditionType: 'QUANTITY_TIERED',
      actionValue: '',
      tiers: [{ fromQuantity: '10', toQuantity: '', value: '5' }],
    });
    expect(bodySent()).toMatchObject({
      action_value: undefined,
      tiers: [{ from_quantity: '10', to_quantity: undefined, value: '5' }],
    });
    // The form hides a target's minimum on a tiered rule, so none is sent: the engine would apply it.
    expect((bodySent().conditions as { min_quantity?: string }[])[1]?.min_quantity).toBeUndefined();

    vi.mocked(apiRequest).mockClear();
    await createPromotion({ ...VALUES, conditionType: 'ON_TOTAL', minTotal: '50000' });
    expect(bodySent()).toMatchObject({ min_total: '50000', conditions: [], tiers: [] });
  });

  it('refuses invalid values before calling the API', async () => {
    await expect(createPromotion({ ...VALUES, name: '' })).resolves.toEqual({
      error: 'INVALID_BODY',
    });
    expect(apiRequest).not.toHaveBeenCalled();
  });

  it('returns the code of an API refusal', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('INVALID_INPUT', 422));
    await expect(createPromotion(VALUES)).resolves.toEqual({ error: 'INVALID_INPUT' });
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe('updatePromotion', () => {
  it('replaces the rule by id', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ id: 'p9' });
    await updatePromotion('p9', VALUES);
    expect(vi.mocked(apiRequest).mock.calls[0]?.[0]).toMatchObject({
      path: '/v1/promotions/p9',
      method: 'PUT',
    });
  });
});

describe('setPromotionActive', () => {
  const stored: Promotion = {
    id: 'p9',
    branchId: 'b1',
    name: 'Escalonada',
    description: null,
    conditionType: 'QUANTITY_TIERED',
    actionType: 'PERCENTAGE',
    actionValue: '0',
    minTotal: null,
    validFrom: '2026-12-01T03:00:00Z',
    validTo: '2027-01-01T02:59:59Z',
    isActive: true,
    isExclusive: true,
    priority: 2,
    conditions: [{ productId: null, familyId: 'f1', subgroupId: null, minQuantity: null }],
    tiers: [{ fromQuantity: '10', toQuantity: null, value: '5' }],
  };

  it('resends the whole rule with only the flag changed', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ id: 'p9' });

    await expect(setPromotionActive(stored, false)).resolves.toEqual({ ok: true, id: 'p9' });
    expect(bodySent()).toEqual({
      name: 'Escalonada',
      description: undefined,
      branch_id: 'b1',
      condition_type: 'QUANTITY_TIERED',
      action_type: 'PERCENTAGE',
      action_value: undefined,
      min_total: undefined,
      valid_from: '2026-12-01T03:00:00Z',
      valid_to: '2027-01-01T02:59:59Z',
      is_active: false,
      is_exclusive: true,
      priority: 2,
      conditions: [
        { product_id: undefined, family_id: 'f1', subgroup_id: undefined, min_quantity: undefined },
      ],
      tiers: [{ from_quantity: '10', to_quantity: undefined, value: '5' }],
    });
  });
});
