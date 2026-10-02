import { render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { makeQuotePayload } from '@/lib/api/public-quotes.fixtures';
import { getFormatters } from '@/lib/i18n/formatters-server';
import { QuoteSummary } from './quote-summary';

vi.mock('next-intl/server', async () => import('@/lib/i18n/server-intl.mock'));

describe('QuoteSummary', () => {
  // Currency comes back with a no-break space that jsdom keeps where its text matcher collapses it.
  function money(fmt: Awaited<ReturnType<typeof getFormatters>>, currency: string, amount: string) {
    return fmt.currency(amount, currency).replace(/\u00a0/g, ' ');
  }

  it('derives the subtotal from the lines, stacks the discounts and states the frozen total', async () => {
    const payload = makeQuotePayload();
    const fmt = await getFormatters();

    const view = render(
      await QuoteSummary({
        items: payload.items,
        discounts: payload.discounts,
        total: payload.total,
        currency: payload.currency,
      }),
    );

    // 220.00 + 150.00, the deterministic display sum of the frozen decimal strings.
    expect(view.getByText(money(fmt, payload.currency, '370.00'))).toBeTruthy();
    expect(view.getByText(payload.discounts[0]!.description)).toBeTruthy();
    expect(
      view.getByText(money(fmt, payload.currency, `-${payload.discounts[0]!.amount}`)),
    ).toBeTruthy();
    expect(view.getByText(money(fmt, payload.currency, payload.total))).toBeTruthy();
  });

  it('renders no discount rows when there are none', async () => {
    const payload = makeQuotePayload({ discounts: [] });
    const fmt = await getFormatters();

    const view = render(
      await QuoteSummary({
        items: payload.items,
        discounts: [],
        total: payload.total,
        currency: payload.currency,
      }),
    );

    expect(view.queryByText('Bonificación')).toBeNull();
    expect(view.getByText(money(fmt, payload.currency, payload.total))).toBeTruthy();
  });
});
