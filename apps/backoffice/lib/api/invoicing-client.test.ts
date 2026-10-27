import { afterEach, describe, expect, it, vi } from 'vitest';

import { recoverInvoice } from '@/lib/api/invoicing-client';

afterEach(() => vi.unstubAllGlobals());

describe('recoverInvoice', () => {
  it("posts to the invoice's own branch and maps what comes back", async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            id: 'inv-1',
            branch_id: 'b-1',
            status: 'ISSUED',
            type: 'B',
            point_of_sale: 3,
            number: 12,
            issued_on: '2026-10-05',
            cae: '74123456789012',
            cae_expires_on: '2026-10-15',
            issuer_cuit: '30712345679',
            receiver: { name: 'Obra Norte', doc_type: 'CUIT', doc_number: '1', iva_condition: 'X' },
            amounts: { net: '1', exempt: '0', vat: '0', total: '1', by_rate: null },
            currency: 'ARS',
            issues: null,
            qr_url: '',
            created_at: '2026-10-05T00:00:00Z',
          }),
        ),
    );
    vi.stubGlobal('fetch', fetchMock);

    const invoice = await recoverInvoice('inv-1', 'b-1');

    expect(fetchMock).toHaveBeenCalledWith('/api/invoices/inv-1/recover', {
      method: 'POST',
      headers: { 'X-Branch-Id': 'b-1' },
      cache: 'no-store',
    });
    expect(invoice).toMatchObject({ id: 'inv-1', branchId: 'b-1', status: 'ISSUED', issues: [] });
  });

  it('throws the refusal by its code', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () => new Response(JSON.stringify({ code: 'INVOICE_IN_PROGRESS' }), { status: 409 }),
      ),
    );

    await expect(recoverInvoice('inv-1', 'b-1')).rejects.toMatchObject({
      code: 'INVOICE_IN_PROGRESS',
    });
  });
});
