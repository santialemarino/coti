import { beforeEach, describe, expect, it, vi } from 'vitest';

import { getCombo, getCombos } from '@/lib/api/combos';
import { ApiError } from '@/lib/api/errors';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));

const { apiRequest } = await import('@/lib/api/client');

beforeEach(() => vi.clearAllMocks());

describe('combos API', () => {
  it('maps every field to camelCase and reads absent lists as empty', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      items: [
        {
          id: 'c1',
          name: 'Obra gruesa',
          description: 'Para obra',
          is_active: true,
          items: [
            {
              product_id: 'p1',
              quantity: '10.00',
              product_code: 'CEM',
              product_name: 'Cemento',
              product_unit: 'bolsa',
              product_is_active: true,
            },
          ],
          branch_ids: ['b1'],
        },
        {
          id: 'c2',
          name: 'Vacío',
          description: null,
          is_active: false,
          items: null,
          branch_ids: null,
        },
      ],
    });
    await expect(getCombos()).resolves.toEqual([
      {
        id: 'c1',
        name: 'Obra gruesa',
        description: 'Para obra',
        isActive: true,
        items: [
          {
            productId: 'p1',
            quantity: '10.00',
            productCode: 'CEM',
            productName: 'Cemento',
            productUnit: 'bolsa',
            productIsActive: true,
          },
        ],
        branchIds: ['b1'],
      },
      { id: 'c2', name: 'Vacío', description: null, isActive: false, items: [], branchIds: [] },
    ]);
  });

  it('answers null for a combo the account does not have', async () => {
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('NOT_FOUND', 404));
    await expect(getCombo('missing')).resolves.toBeNull();
  });
});
