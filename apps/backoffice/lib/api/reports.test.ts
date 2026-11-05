import { beforeEach, describe, expect, it, vi } from 'vitest';

import { apiRequest } from '@/lib/api/client';
import { getSellerReport } from '@/lib/api/reports';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));

function rawReport(overrides: Record<string, unknown> = {}) {
  return {
    range: { from: '2026-10-01', to: '2026-10-02' },
    scope: { multi_branch: false, branch_id: 'branch-1' },
    filters: {
      statuses: [] as string[],
      channel: null as string | null,
      seller_id: null as string | null,
      seller_name: '',
      client_id: null as string | null,
      client_name: '',
    },
    kpis: {
      rfqs_received: 0,
      quotes_sent: 0,
      quotes_accepted: 0,
      conversion_rate: null,
      accepted_amount: '0.00',
      response_time: { average_seconds: null, median_seconds: null, sample: 0 },
    },
    branches: [],
    quotes_by_status: [],
    timeline: [],
    top_products: [],
    top_revenue_products: [],
    top_clients: [],
    ai_match: {
      matched: 0,
      total: 0,
      rate: null,
      confidence: { low: 0, medium: 0, high: 0 },
    },
    ...overrides,
  };
}

describe('getSellerReport', () => {
  beforeEach(() => vi.clearAllMocks());

  it('maps every applied filter returned by the API', async () => {
    vi.mocked(apiRequest).mockResolvedValue(
      rawReport({
        filters: {
          statuses: ['SENT', 'ACCEPTED'],
          channel: 'EMAIL',
          seller_id: 'seller-1',
          seller_name: 'Ana',
          client_id: 'client-1',
          client_name: 'Obra Norte',
        },
      }) as never,
    );

    const report = await getSellerReport({
      from: '2026-10-01',
      to: '2026-10-02',
      statuses: ['SENT', 'ACCEPTED'],
      channel: 'EMAIL',
      sellerId: 'seller-1',
      clientId: 'client-1',
    });

    expect(report.filters).toEqual({
      statuses: ['SENT', 'ACCEPTED'],
      channel: 'EMAIL',
      sellerId: 'seller-1',
      sellerName: 'Ana',
      clientId: 'client-1',
      clientName: 'Obra Norte',
    });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/reports',
      query: {
        from: '2026-10-01',
        to: '2026-10-02',
        status: 'SENT,ACCEPTED',
        channel: 'EMAIL',
        seller_id: 'seller-1',
        client_id: 'client-1',
      },
    });
  });

  it('maps an unfiltered report to an empty filters object', async () => {
    vi.mocked(apiRequest).mockResolvedValue(rawReport() as never);

    const report = await getSellerReport({ from: '2026-10-01', to: '2026-10-02' });

    expect(report.filters).toEqual({
      statuses: [],
      channel: null,
      sellerId: null,
      sellerName: '',
      clientId: null,
      clientName: '',
    });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/reports',
      query: {
        from: '2026-10-01',
        to: '2026-10-02',
        status: undefined,
        channel: undefined,
        seller_id: undefined,
        client_id: undefined,
      },
    });
  });
});
