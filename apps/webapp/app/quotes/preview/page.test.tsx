import { render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { PublicQuoteSend } from '@/lib/api/public-quotes';
import QuotePreviewPage from './page';

const publicQuoteView = vi.hoisted(() => vi.fn());

/*
 * The page reuses PublicQuoteView, an async server component the client renderer cannot await — so
 * this spec stubs it and asserts the fresh SENT snapshot the page feeds it.
 */
vi.mock('@/app/quotes/_components/public-quote-view', () => ({
  PublicQuoteView: ({
    send,
    selectionPreviews,
  }: {
    send: { status: string };
    selectionPreviews?: Record<
      string,
      { quote: { total: string; discounts: { description: string }[] } }
    >;
  }) => {
    publicQuoteView({ send, selectionPreviews });
    return <p data-testid="public-quote-view">{send.status}</p>;
  },
}));

describe('QuotePreviewPage', () => {
  it('feeds the shared view a fresh, open SENT snapshot', async () => {
    const view = render(await QuotePreviewPage());

    const { send, selectionPreviews } = vi.mocked(publicQuoteView).mock.calls[0]![0] as {
      send: PublicQuoteSend;
      selectionPreviews: Record<
        string,
        { quote: { total: string; discounts: { description: string }[] } }
      >;
    };
    expect(send.status).toBe('ACTIVE');
    expect(send.quote?.reference).toBe('COT-000008');
    expect(send.quote?.versionNumber).toBe(1);
    expect(send.quote?.customerName).toBe('Constructora del Riachuelo');
    expect(send.quote?.supplier.name).toBe('Corralón San Martín');
    expect(send.quote?.supplier.legalName).toBe('Corralón San Martín S.R.L.');
    expect(send.quote?.supplier.taxId).toBe('30-71234567-9');
    expect(send.quote?.supplier.brandColor).toBe('#C2410C');
    expect(send.quote?.supplier.logoUrl).toBeUndefined();
    expect(send.quote?.branch).toEqual({
      name: 'Morón',
      address: 'Rivadavia 18400, Morón',
    });
    expect(send.quote?.items).toHaveLength(5);
    expect(send.quote?.items.map((item) => item.productCode)).toEqual([
      'CEM-01',
      'ARE-FIN-01',
      'HIER-08',
      'MEMB-ASF-04',
      'HIDR-01',
    ]);
    expect(send.quote?.items[0]).toMatchObject({
      requestedDescription: '80 bolsas de cemento portland',
      productName: 'Cemento Portland CP40 50 kg',
      quantity: '80.00',
      unit: 'bolsa',
      unitPrice: '9500.00',
      subtotal: '760000.00',
    });
    expect(send.quote?.items[1]?.alternatives[0]?.unitPrice).toBe('35000.00');
    expect(send.quote?.discounts).toEqual([
      { description: 'Bonificación por pronto pago', amount: '30800.00' },
    ]);
    expect(send.quote?.total).toBe('1340000.00');
    expect(Object.values(selectionPreviews).map((preview) => preview.quote.total)).toEqual([
      '995900.00',
      '1375200.00',
      '1031100.00',
    ]);
    expect(
      Object.values(selectionPreviews).every(
        (preview) =>
          preview.quote.discounts[0]?.description === 'Bonificaci\u00f3n por pronto pago',
      ),
    ).toBe(true);
    expect(send.pdfUrl).toBeTruthy();
    expect(Date.parse(send.quote?.approvedAt ?? '')).toBeLessThan(Date.now());
    expect(Date.parse(send.expiresAt)).toBeGreaterThan(Date.now());
    expect(view.getByTestId('public-quote-view')).toBeTruthy();
  });
});
