import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  createCombo,
  setComboActive,
  updateCombo,
} from '@/app/(protected)/settings/combos/actions';
import type { Combo } from '@/lib/api/combos';

vi.mock('next/cache', () => ({ revalidatePath: vi.fn() }));
vi.mock('@/lib/api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/client')>()),
  apiRequest: vi.fn(),
}));

const { revalidatePath } = await import('next/cache');
const { apiRequest } = await import('@/lib/api/client');
const { ApiError } = await import('@/lib/api/errors');

const VALUES = {
  name: 'Obra gruesa',
  description: '',
  isActive: true,
  items: [
    { productId: 'p1', label: 'Cemento', quantity: '10' },
    { productId: 'p2', label: 'Arena', quantity: '2.5' },
  ],
  branchIds: ['b1', 'b2'],
};

function sent() {
  return vi.mocked(apiRequest).mock.calls[0]?.[0];
}

beforeEach(() => vi.clearAllMocks());

describe('combo actions', () => {
  it('creates the combo in the API shape and revalidates the list', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ id: 'c1' });
    await expect(createCombo(VALUES)).resolves.toEqual({ ok: true });
    expect(sent()).toEqual({
      path: '/v1/combos',
      method: 'POST',
      body: {
        name: 'Obra gruesa',
        description: undefined,
        is_active: true,
        items: [
          { product_id: 'p1', quantity: '10' },
          { product_id: 'p2', quantity: '2.5' },
        ],
        branch_ids: ['b1', 'b2'],
      },
    });
    expect(revalidatePath).toHaveBeenCalledWith('/settings/combos', 'layout');
  });

  it('refuses invalid values before calling the API and returns API refusals', async () => {
    await expect(createCombo({ ...VALUES, items: [] })).resolves.toEqual({ error: 'INVALID_BODY' });
    expect(apiRequest).not.toHaveBeenCalled();
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('INVALID_INPUT', 422));
    await expect(updateCombo('c1', VALUES)).resolves.toEqual({ error: 'INVALID_INPUT' });
    expect(sent()).toMatchObject({ path: '/v1/combos/c1', method: 'PUT' });
  });

  it('switches a combo off by resending it whole', async () => {
    vi.mocked(apiRequest).mockResolvedValue({ id: 'c1' });
    const combo: Combo = {
      id: 'c1',
      name: 'Obra gruesa',
      description: null,
      isActive: true,
      items: [
        {
          productId: 'p1',
          quantity: '10.00',
          productCode: null,
          productName: 'Cemento',
          productUnit: null,
          productIsActive: true,
        },
        {
          productId: 'p2',
          quantity: '2.00',
          productCode: null,
          productName: 'Arena',
          productUnit: null,
          productIsActive: true,
        },
      ],
      branchIds: ['b1'],
    };
    await setComboActive(combo, false);
    expect(sent()?.body).toEqual({
      name: 'Obra gruesa',
      description: undefined,
      is_active: false,
      items: [
        { product_id: 'p1', quantity: '10.00' },
        { product_id: 'p2', quantity: '2.00' },
      ],
      branch_ids: ['b1'],
    });
  });
});
