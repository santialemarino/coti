import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '@/lib/api/errors';
import { getPromotion, getPromotions } from '@/lib/api/promotions';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));

const { apiRequest } = await import('@/lib/api/client');

function rawPromotion(overrides: Record<string, unknown> = {}) {
  return {
    id: 'p1',
    branch_id: 'b1',
    name: 'Cemento por volumen',
    description: 'Solo obra',
    condition_type: 'QUANTITY_TIERED',
    action_type: 'PERCENTAGE',
    action_value: '0',
    min_total: null,
    valid_from: '2026-11-01T03:00:00Z',
    valid_to: null,
    is_active: true,
    is_exclusive: false,
    priority: 5,
    conditions: [{ product_id: 'prod-1', family_id: null, subgroup_id: null, min_quantity: '2' }],
    tiers: [{ from_quantity: '10', to_quantity: '19', value: '5' }],
    created_at: '2026-11-01T10:00:00Z',
    updated_at: '2026-11-01T10:00:00Z',
    ...overrides,
  };
}

beforeEach(() => vi.clearAllMocks());

describe('getPromotions', () => {
  it('turns every snake_case field into its camelCase counterpart', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ items: [rawPromotion()] });

    await expect(getPromotions()).resolves.toEqual([
      {
        id: 'p1',
        branchId: 'b1',
        name: 'Cemento por volumen',
        description: 'Solo obra',
        conditionType: 'QUANTITY_TIERED',
        actionType: 'PERCENTAGE',
        actionValue: '0',
        minTotal: null,
        validFrom: '2026-11-01T03:00:00Z',
        validTo: null,
        isActive: true,
        isExclusive: false,
        priority: 5,
        conditions: [{ productId: 'prod-1', familyId: null, subgroupId: null, minQuantity: '2' }],
        tiers: [{ fromQuantity: '10', toQuantity: '19', value: '5' }],
      },
    ]);
    expect(apiRequest).toHaveBeenCalledWith({ path: '/v1/promotions', branchScoped: false });
  });

  it('reads absent lists as empty', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      items: [rawPromotion({ conditions: null, tiers: null })],
    });

    const [promotion] = await getPromotions();
    expect(promotion?.conditions).toEqual([]);
    expect(promotion?.tiers).toEqual([]);
  });
});

describe('getPromotion', () => {
  it('answers null for a rule the account does not have', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('NOT_FOUND', 404));
    await expect(getPromotion('missing')).resolves.toBeNull();
  });

  it('answers null for a malformed id', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('INVALID_INPUT', 400));
    await expect(getPromotion('not-a-uuid')).resolves.toBeNull();
  });

  it('rethrows any other failure', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('UNREACHABLE', 0));
    await expect(getPromotion('p1')).rejects.toBeInstanceOf(ApiError);
  });
});
