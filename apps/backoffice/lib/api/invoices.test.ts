import { beforeEach, describe, expect, it, vi } from 'vitest';

import { getSalesAwaitingInvoice } from '@/lib/api/invoices';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));

const { apiRequest } = await import('@/lib/api/client');

beforeEach(() => vi.clearAllMocks());

describe('getSalesAwaitingInvoice', () => {
  it('asks for the page and maps every field of every sale', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      items: [
        {
          quote_id: 'q1',
          rfq_id: 'r1',
          quote_number: 42,
          branch_id: 'b1',
          branch_name: 'Morón',
          client_id: null,
          client: 'Obra Norte',
          total: '1210.00',
          accepted_at: '2026-10-05T12:00:00Z',
          paid_at: '2026-10-06T12:00:00Z',
        },
      ],
      total: 26,
      page: 2,
      page_size: 25,
    });

    await expect(getSalesAwaitingInvoice(2)).resolves.toEqual({
      items: [
        {
          quoteId: 'q1',
          rfqId: 'r1',
          quoteNumber: 42,
          branchId: 'b1',
          branchName: 'Morón',
          clientId: null,
          client: 'Obra Norte',
          total: '1210.00',
          acceptedAt: '2026-10-05T12:00:00Z',
          paidAt: '2026-10-06T12:00:00Z',
        },
      ],
      total: 26,
      page: 2,
      pageSize: 25,
    });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/quotes/awaiting-invoice',
      query: { page: '2' },
    });
  });
});
