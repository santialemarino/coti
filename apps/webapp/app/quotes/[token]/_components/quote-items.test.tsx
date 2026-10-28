import { fireEvent, render, waitFor, within } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import { makeQuotePayload } from '@/lib/api/public-quotes.fixtures';
import messages from '@/translations/es.json';
import { QuoteItems } from './quote-items';
import { QuoteSelectionProvider } from './quote-selection-context';

function renderItems(props: React.ComponentProps<typeof QuoteItems>) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <QuoteSelectionProvider>
        <QuoteItems {...props} />
      </QuoteSelectionProvider>
    </NextIntlClientProvider>,
  );
}

describe('QuoteItems', () => {
  it('keeps an approved alternative under its own quoted line', () => {
    const quote = makeQuotePayload();
    const view = renderItems({
      items: quote.items,
      discounts: quote.discounts,
      total: quote.total,
      currency: quote.currency,
    });

    const desktop = within(view.getByTestId('items-table-desktop'));
    const alternative = desktop.getByText('Cemento Portland 25kg').closest('tr')!;
    expect(within(alternative).getByText(messages.quote.options.economy)).toBeTruthy();
    expect(alternative.getAttribute('role')).toBe('button');
    expect(view.queryByText(messages.quote.options.heading)).toBeNull();
  });

  it('selects the inline option and replaces the summary with the backend preview', async () => {
    const quote = makeQuotePayload();
    const preview = {
      quote: {
        ...quote,
        discounts: [{ description: 'Descuento recalculado', amount: '10.00' }],
        total: '260.00',
      },
      subtotal: '270.00',
    };
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => preview });
    vi.stubGlobal('fetch', fetchMock);
    const view = renderItems({
      items: quote.items,
      discounts: quote.discounts,
      total: quote.total,
      currency: quote.currency,
      token: 'tok-abc',
    });

    const button = view.getAllByRole('button', {
      name: `Elegir ${quote.items[0]!.alternatives[0]!.name}`,
    })[0]!;
    fireEvent.click(button);
    expect(view.getByText(messages.quote.recalculating)).toBeTruthy();

    await waitFor(() => expect(view.getByText('Descuento recalculado')).toBeTruthy());
    expect(button.closest('tr')?.dataset.state).toBe('selected');
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(JSON.parse((fetchMock.mock.calls[0]![1] as RequestInit).body as string)).toEqual({
      token: 'tok-abc',
      selections: [
        {
          item_id: quote.items[0]!.id,
          alternative_id: quote.items[0]!.alternatives[0]!.id,
        },
      ],
    });
    vi.unstubAllGlobals();
  });

  it('restores the original summary when the chosen inline option is clicked again', async () => {
    const quote = makeQuotePayload();
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ quote: { ...quote, total: '260.00' }, subtotal: '270.00' }),
    });
    vi.stubGlobal('fetch', fetchMock);
    const view = renderItems({
      items: quote.items,
      discounts: quote.discounts,
      total: quote.total,
      currency: quote.currency,
      token: 'tok-abc',
    });

    const choose = view.getAllByRole('button', {
      name: `Elegir ${quote.items[0]!.alternatives[0]!.name}`,
    })[0]!;
    fireEvent.click(choose);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    fireEvent.click(view.getAllByRole('button', { name: messages.quote.restoreQuotedOption })[0]!);

    await waitFor(() => expect(view.queryByText(messages.quote.recalculating)).toBeNull());
    expect(fetchMock).toHaveBeenCalledTimes(1);
    vi.unstubAllGlobals();
  });
});
