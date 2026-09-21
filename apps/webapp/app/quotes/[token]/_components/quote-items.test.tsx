import { render, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { makeQuotePayload } from '@/lib/api/public-quotes.fixtures';
import { getFormatters } from '@/lib/i18n/formatters-server';
import { QuoteItems } from './quote-items';

vi.mock('next-intl/server', async () => import('@/lib/i18n/server-intl.mock'));

/*
 * QuoteItems renders the financial summary as its async footer, which the jsdom renderer cannot
 * await in the middle of the tree — the mock keeps this spec on the items, and quote-summary.test
 * covers the footer on its own.
 */
vi.mock('./quote-summary', () => ({
  QuoteSummary: () => <p data-testid="quote-summary" />,
}));

describe('QuoteItems', () => {
  /*
   * Currency comes back with a no-break space that jsdom keeps where its text matcher collapses it,
   * so the expected strings are normalised the same way. The same line renders twice — once in the
   * desktop table, once in the mobile blocks — each `display:none` at the other breakpoint, so the
   * queries below scope against the visible-for-each variant.
   */
  function money(fmt: Awaited<ReturnType<typeof getFormatters>>, currency: string, amount: string) {
    return fmt.currency(amount, currency).replace(/\u00a0/g, ' ');
  }

  it('lists each line in the desktop table with quantity, unit, price and subtotal', async () => {
    const payload = makeQuotePayload();
    const fmt = await getFormatters();

    const view = render(
      await QuoteItems({
        items: payload.items,
        discounts: payload.discounts,
        total: payload.total,
        currency: payload.currency,
      }),
    );

    const desktop = within(view.getByTestId('items-table-desktop'));
    const cementRow = desktop.getByText('Cemento Portland 50kg').closest('tr')!;
    expect(within(cementRow).getByText('CEM-01')).toBeTruthy();
    expect(within(cementRow).getByText('Pediste: dos bolsas de cemento')).toBeTruthy();
    expect(within(cementRow).getByText('2')).toBeTruthy();
    expect(within(cementRow).getByText('bolsa')).toBeTruthy();
    expect(within(cementRow).getByText(money(fmt, payload.currency, '110.00'))).toBeTruthy();
    expect(within(cementRow).getByText(money(fmt, payload.currency, '220.00'))).toBeTruthy();

    const calRow = desktop.getByText('Cal común').closest('tr')!;
    expect(within(calRow).getByText('10')).toBeTruthy();
    expect(within(calRow).getByText(money(fmt, payload.currency, '15.00'))).toBeTruthy();
    expect(within(calRow).getByText(money(fmt, payload.currency, '150.00'))).toBeTruthy();
  });

  it('renders the same lines as compact blocks on a phone', async () => {
    const payload = makeQuotePayload();
    const fmt = await getFormatters();

    const view = render(
      await QuoteItems({
        items: payload.items,
        discounts: payload.discounts,
        total: payload.total,
        currency: payload.currency,
      }),
    );

    const mobile = within(view.getByTestId('items-list-mobile'));
    const cementBlock = mobile.getByText('Cemento Portland 50kg').closest('li')!;
    expect(within(cementBlock).getByText('2 bolsa')).toBeTruthy();
    expect(within(cementBlock).getByText('Pediste: dos bolsas de cemento')).toBeTruthy();
    expect(within(cementBlock).getByText(money(fmt, payload.currency, '110.00'))).toBeTruthy();
    expect(within(cementBlock).getByText(money(fmt, payload.currency, '220.00'))).toBeTruthy();

    // The two breakpoints are one shared document, not duplicates inside it.
    expect(view.getAllByText('Cemento Portland 50kg')).toHaveLength(2);
  });

  /* An approved alternative sits under the line it offers, priced in the same column. */
  it('places each alternative under its own line with its own price', async () => {
    const payload = makeQuotePayload();
    const fmt = await getFormatters();

    const view = render(
      await QuoteItems({
        items: payload.items,
        discounts: payload.discounts,
        total: payload.total,
        currency: payload.currency,
      }),
    );

    const desktop = within(view.getByTestId('items-table-desktop'));
    const alternativeRow = desktop.getByText('Cemento Portland 25kg').closest('tr')!;
    expect(within(alternativeRow).getByText('Alternativa')).toBeTruthy();
    expect(within(alternativeRow).getByText(money(fmt, payload.currency, '60.00'))).toBeTruthy();

    const mobile = within(view.getByTestId('items-list-mobile'));
    const cementBlock = mobile.getByText('Cemento Portland 50kg').closest('li')!;
    expect(within(cementBlock).getByText('Alternativa')).toBeTruthy();
    expect(within(cementBlock).getByText('Cemento Portland 25kg')).toBeTruthy();
    expect(within(cementBlock).getByText(money(fmt, payload.currency, '60.00'))).toBeTruthy();
  });

  it('closes the card with the financial summary slot', async () => {
    const payload = makeQuotePayload();

    const view = render(
      await QuoteItems({
        items: payload.items,
        discounts: payload.discounts,
        total: payload.total,
        currency: payload.currency,
      }),
    );

    expect(view.getByTestId('quote-summary')).toBeTruthy();
  });

  it('reports an empty quote instead of a bare heading', async () => {
    const view = render(
      await QuoteItems({ items: [], discounts: [], total: '0.00', currency: 'ARS' }),
    );

    expect(view.getByText('Esta cotización todavía no tiene ítems.')).toBeTruthy();
  });
});
