import { fireEvent, render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { makeQuotePayload } from '@/lib/api/public-quotes.fixtures';
import messages from '@/translations/es.json';
import { QuoteOptionSelector } from './quote-option-selector';
import { QuoteSelectionProvider, useQuoteSelections } from './quote-selection-context';

function SelectionProbe() {
  const { selections } = useQuoteSelections();
  return <output>{JSON.stringify(selections)}</output>;
}

describe('QuoteOptionSelector', () => {
  it('labels the economy option and sends only the elected alternative onward', () => {
    const quote = makeQuotePayload();
    const item = quote.items[0]!;
    const alternative = item.alternatives[0]!;
    const view = render(
      <NextIntlClientProvider
        locale="es"
        messages={messages}
        timeZone="America/Argentina/Buenos_Aires"
      >
        <QuoteSelectionProvider>
          <QuoteOptionSelector items={quote.items} currency={quote.currency} />
          <SelectionProbe />
        </QuoteSelectionProvider>
      </NextIntlClientProvider>,
    );

    expect(view.getByText(messages.quote.options.economy)).toBeTruthy();
    fireEvent.click(view.getByRole('radio', { name: new RegExp(alternative.name) }));
    expect(
      view.getByText(JSON.stringify([{ itemId: item.id, alternativeId: alternative.id }])),
    ).toBeTruthy();

    fireEvent.click(view.getByRole('radio', { name: new RegExp(item.productName) }));
    expect(view.getByText('[]')).toBeTruthy();
  });
});
