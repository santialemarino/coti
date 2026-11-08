import type { ReactNode } from 'react';
import { act, fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { QuoteActions } from '@/app/quotes/[token]/_components/quote-actions';
import { QuoteItems } from '@/app/quotes/[token]/_components/quote-items';
import {
  QuoteSelectionProvider,
  useQuoteSelections,
} from '@/app/quotes/[token]/_components/quote-selection-context';
import type { PublicQuoteSelectionPreview, QuotePayload } from '@/lib/api/public-quotes';
import { makeQuotePayload } from '@/lib/api/public-quotes.fixtures';
import messages from '@/translations/es.json';

afterEach(() => vi.unstubAllGlobals());

function renderQuote(quote = makeQuotePayload(), controls?: ReactNode) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <QuoteSelectionProvider>
        {controls}
        <QuoteItems
          items={quote.items}
          discounts={quote.discounts}
          total={quote.total}
          currency={quote.currency}
          token="tok-abc"
        />
        <QuoteActions token="tok-abc" />
      </QuoteSelectionProvider>
    </NextIntlClientProvider>,
  );
}

function SelectionControl({ itemId }: { itemId: string }) {
  const { select } = useQuoteSelections();
  return (
    <button type="button" onClick={() => select(itemId, 'premium-option')}>
      Update selection
    </button>
  );
}

function deferredPreview() {
  let resolve!: (value: Response) => void;
  const promise = new Promise<Response>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function previewResponse(quote: QuotePayload, total = '260.00') {
  const preview: PublicQuoteSelectionPreview = {
    quote: { ...quote, total },
    subtotal: '280.00',
  };
  return new Response(JSON.stringify(preview), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}

describe('quote selection acceptance', () => {
  it('blocks acceptance while the selected combination has no pricing preview', () => {
    const pending = deferredPreview();
    const fetchMock = vi.fn().mockReturnValue(pending.promise);
    vi.stubGlobal('fetch', fetchMock);
    const quote = makeQuotePayload();
    const view = renderQuote(quote);

    fireEvent.click(
      view.getAllByRole('button', { name: `Elegir ${quote.items[0]!.alternatives[0]!.name}` })[0]!,
    );

    expect(view.getByText(messages.quote.recalculating)).toBeTruthy();
    expect(view.getByText(/350,00/)).toBeTruthy();
    const accept = view.getByRole('button', {
      name: messages.quote.actions.acceptLabel,
    }) as HTMLButtonElement;
    expect(accept.disabled).toBe(true);
    fireEvent.click(accept);
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(['/api/public-quote-preview']);
    expect(view.queryByRole('dialog')).toBeNull();
  });

  it('keeps acceptance blocked after a failed preview and allows restoring the base offer', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 502 }));
    vi.stubGlobal('fetch', fetchMock);
    const quote = makeQuotePayload();
    const view = renderQuote(quote);

    fireEvent.click(
      view.getAllByRole('button', { name: `Elegir ${quote.items[0]!.alternatives[0]!.name}` })[0]!,
    );
    await waitFor(() => expect(view.getByText(messages.quote.recalculationError)).toBeTruthy());

    const accept = view.getByRole('button', {
      name: messages.quote.actions.acceptLabel,
    }) as HTMLButtonElement;
    expect(accept.disabled).toBe(true);
    fireEvent.click(view.getAllByRole('button', { name: messages.quote.restoreQuotedOption })[0]!);
    expect(accept.disabled).toBe(false);
    expect(view.queryByText(messages.quote.recalculationError)).toBeNull();

    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ customerStatus: 'ACCEPT', createdAt: '2026-11-07T12:00:00Z' })),
    );
    fireEvent.click(accept);
    fireEvent.click(view.getByRole('button', { name: messages.quote.actions.confirm }));
    await waitFor(() => expect(view.getByText(messages.quote.actions.resultAccept)).toBeTruthy());
    expect(JSON.parse((fetchMock.mock.calls[1]![1] as RequestInit).body as string)).toEqual({
      token: 'tok-abc',
      type: 'ACCEPT',
    });
  });

  it('accepts a selected alternative after its preview updates the displayed price', async () => {
    const quote = makeQuotePayload();
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(previewResponse(quote))
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ customerStatus: 'ACCEPT', createdAt: '2026-11-07T12:00:00Z' }),
        ),
      );
    vi.stubGlobal('fetch', fetchMock);
    const view = renderQuote(quote);

    fireEvent.click(
      view.getAllByRole('button', { name: `Elegir ${quote.items[0]!.alternatives[0]!.name}` })[0]!,
    );
    await waitFor(() => expect(view.getByText(/260/)).toBeTruthy());
    const accept = view.getByRole('button', {
      name: messages.quote.actions.acceptLabel,
    }) as HTMLButtonElement;
    expect(accept.disabled).toBe(false);
    fireEvent.click(accept);
    fireEvent.click(view.getByRole('button', { name: messages.quote.actions.confirm }));

    await waitFor(() => expect(view.getByText(messages.quote.actions.resultAccept)).toBeTruthy());
    expect(JSON.parse((fetchMock.mock.calls[1]![1] as RequestInit).body as string)).toEqual({
      token: 'tok-abc',
      type: 'ACCEPT',
      selections: [
        { item_id: quote.items[0]!.id, alternative_id: quote.items[0]!.alternatives[0]!.id },
      ],
    });
  });

  it('ignores an old preview when another selection is still being priced', async () => {
    const quote = makeQuotePayload();
    quote.items[0]!.alternatives.push({
      ...quote.items[0]!.alternatives[0]!,
      id: 'premium-option',
      name: 'Cemento premium',
      recommendationType: 'PREMIUM',
      unitPrice: '150.00',
    });
    const first = deferredPreview();
    const second = deferredPreview();
    const fetchMock = vi
      .fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    vi.stubGlobal('fetch', fetchMock);
    const view = renderQuote(quote);
    const accept = view.getByRole('button', {
      name: messages.quote.actions.acceptLabel,
    }) as HTMLButtonElement;

    fireEvent.click(
      view.getAllByRole('button', { name: `Elegir ${quote.items[0]!.alternatives[0]!.name}` })[0]!,
    );
    fireEvent.click(view.getAllByRole('button', { name: 'Elegir Cemento premium' })[0]!);
    await act(async () => first.resolve(previewResponse(quote)));

    expect(accept.disabled).toBe(true);
    expect(view.queryByText(/260/)).toBeNull();
    await act(async () => second.resolve(previewResponse(quote, '430.00')));
    await waitFor(() => expect(view.getByText(/430/)).toBeTruthy());
    expect(accept.disabled).toBe(false);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('revalidates an open acceptance dialog when its selection changes', async () => {
    const quote = makeQuotePayload();
    quote.items[0]!.alternatives.push({
      ...quote.items[0]!.alternatives[0]!,
      id: 'premium-option',
      name: 'Cemento premium',
      recommendationType: 'PREMIUM',
      unitPrice: '150.00',
    });
    const pending = deferredPreview();
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(previewResponse(quote))
      .mockReturnValueOnce(pending.promise)
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ customerStatus: 'ACCEPT', createdAt: '2026-11-07T12:00:00Z' }),
        ),
      );
    vi.stubGlobal('fetch', fetchMock);
    const view = renderQuote(quote, <SelectionControl itemId={quote.items[0]!.id} />);
    const selectionControl = view.getByRole('button', { name: 'Update selection' });

    fireEvent.click(
      view.getAllByRole('button', { name: `Elegir ${quote.items[0]!.alternatives[0]!.name}` })[0]!,
    );
    await waitFor(() => expect(view.getByText(/260/)).toBeTruthy());
    fireEvent.click(view.getByRole('button', { name: messages.quote.actions.acceptLabel }));
    const confirm = view.getByRole('button', {
      name: messages.quote.actions.confirm,
    }) as HTMLButtonElement;

    fireEvent.click(selectionControl);
    expect(confirm.disabled).toBe(true);
    fireEvent.click(confirm);
    expect(fetchMock).toHaveBeenCalledTimes(2);

    await act(async () => pending.resolve(previewResponse(quote, '430.00')));
    await waitFor(() => expect(confirm.disabled).toBe(false));
    fireEvent.click(confirm);
    await waitFor(() => expect(view.getByText(messages.quote.actions.resultAccept)).toBeTruthy());
    expect(JSON.parse((fetchMock.mock.calls[2]![1] as RequestInit).body as string)).toEqual({
      token: 'tok-abc',
      type: 'ACCEPT',
      selections: [{ item_id: quote.items[0]!.id, alternative_id: 'premium-option' }],
    });
  });

  it.each([
    { type: 'REQUEST_CHANGE', previewFails: false },
    { type: 'REQUEST_CHANGE', previewFails: true },
    { type: 'REJECT', previewFails: false },
    { type: 'REJECT', previewFails: true },
  ] as const)(
    'allows $type with a selection when preview failure is $previewFails',
    async ({ type, previewFails }) => {
      const pending = deferredPreview();
      const fetchMock = vi.fn().mockImplementation((url: string) => {
        if (url === '/api/public-quote-preview') {
          return previewFails
            ? Promise.resolve(new Response(null, { status: 502 }))
            : pending.promise;
        }
        return Promise.resolve(
          new Response(JSON.stringify({ customerStatus: type, createdAt: '2026-11-07T12:00:00Z' })),
        );
      });
      vi.stubGlobal('fetch', fetchMock);
      const quote = makeQuotePayload();
      const view = renderQuote(quote);
      fireEvent.click(
        view.getAllByRole('button', {
          name: `Elegir ${quote.items[0]!.alternatives[0]!.name}`,
        })[0]!,
      );
      if (previewFails) {
        await waitFor(() => expect(view.getByText(messages.quote.recalculationError)).toBeTruthy());
      }
      const choice = view.getByRole('button', {
        name:
          type === 'REQUEST_CHANGE'
            ? messages.quote.actions.requestChangesLabel
            : messages.quote.actions.rejectLabel,
      }) as HTMLButtonElement;
      expect(choice.disabled).toBe(false);
      fireEvent.click(choice);
      if (type === 'REQUEST_CHANGE') {
        fireEvent.change(view.getByRole('textbox', { name: messages.quote.actions.messageLabel }), {
          target: { value: 'Necesito otra presentación' },
        });
      }
      fireEvent.click(view.getByRole('button', { name: messages.quote.actions.confirm }));
      await waitFor(() =>
        expect(
          view.getByText(
            type === 'REQUEST_CHANGE'
              ? messages.quote.actions.resultRequestChange
              : messages.quote.actions.resultReject,
          ),
        ).toBeTruthy(),
      );
      expect(JSON.parse((fetchMock.mock.calls[1]![1] as RequestInit).body as string)).toEqual({
        token: 'tok-abc',
        type,
        ...(type === 'REQUEST_CHANGE' ? { message: 'Necesito otra presentación' } : {}),
      });
    },
  );
});
