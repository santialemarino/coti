import { afterEach, describe, expect, it, vi } from 'vitest';

import { catalogCompatibility, searchCatalog } from '@/lib/api/catalog';

afterEach(() => vi.unstubAllGlobals());

describe('catalog recommendation search', () => {
  it('reports a failed page instead of returning an empty result', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false }));
    await expect(
      searchCatalog('', { familyId: 'family', subgroupId: 'subgroup', unit: 'bolsa' }),
    ).rejects.toThrow();
  });
  it('requires a family, subgroup and nonempty unit', () => {
    const product = {
      id: 'base',
      code: 'CAL',
      name: 'Cal',
      familyId: 'family',
      subgroupId: null,
      unit: 'bolsa',
    };
    expect(catalogCompatibility(product)).toBeNull();
    expect(catalogCompatibility({ ...product, subgroupId: 'subgroup', unit: ' ' })).toBeNull();
    expect(catalogCompatibility({ ...product, subgroupId: 'subgroup' })).toEqual({
      familyId: 'family',
      subgroupId: 'subgroup',
      unit: 'bolsa',
    });
  });

  it('searches every matching taxonomy page until a matching unit is found', async () => {
    const page = Array.from({ length: 50 }, (_, i) => ({
      id: `other-${i}`,
      code: null,
      canonical_name: 'Otro',
      unit: 'm3',
      family_id: 'family',
      subgroup_id: 'subgroup',
    }));
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ items: page, total: 51 }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          items: [
            {
              id: 'candidate',
              code: 'CAL',
              canonical_name: 'Cal',
              unit: ' BOLSA ',
              family_id: 'family',
              subgroup_id: 'subgroup',
            },
          ],
          total: 51,
        }),
      });
    vi.stubGlobal('fetch', fetchMock);

    const results = await searchCatalog('', {
      familyId: 'family',
      subgroupId: 'subgroup',
      unit: 'bolsa',
    });
    expect(results.map((product) => product.id)).toEqual(['candidate']);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[0]?.[0]).toContain('family_id=family');
    expect(fetchMock.mock.calls[0]?.[0]).toContain('subgroup_id=subgroup');
    expect(fetchMock.mock.calls[1]?.[0]).toContain('offset=50');
  });
});
