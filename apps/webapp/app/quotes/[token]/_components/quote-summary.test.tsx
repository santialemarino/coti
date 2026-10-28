import { render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { makeQuotePayload } from '@/lib/api/public-quotes.fixtures';
import messages from '@/translations/es.json';
import { QuoteSummary } from './quote-summary';

function renderSummary(props: React.ComponentProps<typeof QuoteSummary>) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <QuoteSummary {...props} />
    </NextIntlClientProvider>,
  );
}

describe('QuoteSummary', () => {
  it('uses the backend subtotal when a selected option was repriced', () => {
    const quote = makeQuotePayload();
    const view = renderSummary({
      items: quote.items,
      discounts: quote.discounts,
      total: '260.00',
      subtotal: '270.00',
      currency: quote.currency,
    });

    expect(view.getByText(/270/)).toBeTruthy();
    expect(view.getByText(/260/)).toBeTruthy();
  });

  it('announces that the authoritative total is being recalculated', () => {
    const quote = makeQuotePayload();
    const view = renderSummary({
      items: quote.items,
      discounts: quote.discounts,
      total: quote.total,
      currency: quote.currency,
      isCalculating: true,
    });

    expect(view.getByText(messages.quote.recalculating)).toBeTruthy();
  });
});
