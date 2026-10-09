import { beforeEach, describe, expect, it, vi } from 'vitest';

import { getSellerReport } from '@/lib/api/reports';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));

const { apiRequest } = await import('@/lib/api/client');

beforeEach(() => vi.clearAllMocks());

describe('getSellerReport', () => {
  it('maps the complete seller report from the API wire shape', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      orders_received: 8,
      quotes_sent: 6,
      quotes_accepted: 3,
      average_quote_time_seconds: 5400,
      statuses: [{ status: 'ACCEPTED', count: 3 }],
      top_materials: [{ name: 'Cemento', order_count: 4 }],
      top_clients: [{ name: 'Obras Norte', order_count: 2 }],
    });

    await expect(
      getSellerReport({ dateFrom: '2026-04-01', dateTo: '2026-04-30' }),
    ).resolves.toEqual({
      ordersReceived: 8,
      quotesSent: 6,
      quotesAccepted: 3,
      averageQuoteTimeSeconds: 5400,
      statuses: [{ status: 'ACCEPTED', count: 3 }],
      topMaterials: [{ name: 'Cemento', orderCount: 4 }],
      topClients: [{ name: 'Obras Norte', orderCount: 2 }],
    });
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/reports',
      query: { date_from: '2026-04-01', date_to: '2026-04-30' },
    });
  });

  it('preserves an unavailable quote time as null', async () => {
    vi.mocked(apiRequest).mockResolvedValue({
      orders_received: 1,
      quotes_sent: 0,
      quotes_accepted: 0,
      average_quote_time_seconds: null,
      statuses: [{ status: 'DRAFT', count: 1 }],
      top_materials: [],
      top_clients: [],
    });

    const report = await getSellerReport({});

    expect(report.averageQuoteTimeSeconds).toBeNull();
    expect(report.topMaterials).toEqual([]);
    expect(report.topClients).toEqual([]);
    expect(apiRequest).toHaveBeenCalledWith({
      path: '/v1/reports',
      query: { date_from: undefined, date_to: undefined },
    });
  });
});
