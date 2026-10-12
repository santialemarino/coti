import { beforeEach, describe, expect, it, vi } from 'vitest';

import { apiRequest } from '@/lib/api/client';
import { getClientFiscal } from '@/lib/api/clients';
import { updateClientFiscal } from '@/lib/api/clients-client';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));
const raw = {
  id: 'c1',
  name: 'Eduardo',
  legal_name: 'Obra SA',
  tax_id: '30701234568',
  address: 'San Martin 123',
  iva_condition: 'REGISTERED',
};
beforeEach(() => vi.clearAllMocks());
describe('client fiscal boundary', () => {
  it('maps the client fiscal read independently of order state', async () => {
    vi.mocked(apiRequest).mockResolvedValue(raw);
    expect(await getClientFiscal('c1')).toEqual({
      id: 'c1',
      name: 'Eduardo',
      legalName: 'Obra SA',
      taxId: '30701234568',
      address: 'San Martin 123',
      ivaCondition: 'REGISTERED',
    });
    expect(apiRequest).toHaveBeenCalledWith({ path: '/v1/clients/c1/fiscal' });
  });
  it('maps camelCase edits to the client API and returns the stored values', async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify(raw), { status: 200 }));
    vi.stubGlobal('fetch', fetch);
    try {
      const saved = await updateClientFiscal('c1', {
        legalName: 'Obra SA',
        taxId: '30701234568',
        address: 'San Martin 123',
        ivaCondition: 'REGISTERED',
      });
      expect(fetch).toHaveBeenCalledWith(
        '/api/clients/c1/fiscal',
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify({
            address: 'San Martin 123',
            legal_name: 'Obra SA',
            tax_id: '30701234568',
            iva_condition: 'REGISTERED',
          }),
        }),
      );
      expect(saved.legalName).toBe('Obra SA');
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
