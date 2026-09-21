import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import messages from '@/translations/es.json';
import { QuoteActions } from './quote-actions';

const copy = messages.quote.actions;

function renderActions(props: React.ComponentProps<typeof QuoteActions>) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <QuoteActions {...props} />
    </NextIntlClientProvider>,
  );
}

function fetchResolving(body: unknown) {
  return vi.fn().mockResolvedValue({
    ok: true,
    json: async () => body,
  });
}

/*
 * The public page's answer flow: an idle state with the three decisions, dialogs that word the
 * consequence before committing, a real round trip through the app route for a token, and a
 * simulated one on the dev-only preview. REQUEST_CHANGE demands a message because the seller has
 * to know what to rework.
 */
describe('QuoteActions', () => {
  it('offers the three decisions when the send has no answer yet', () => {
    const view = renderActions({ token: 'tok-abc' });

    expect(view.getByRole('button', { name: copy.acceptLabel })).toBeTruthy();
    expect(view.getByRole('button', { name: copy.requestChangesLabel })).toBeTruthy();
    expect(view.getByRole('button', { name: copy.rejectLabel })).toBeTruthy();
    expect(view.queryByText(copy.resultTitle)).toBeNull();
  });

  it('posts an accept through the app route and shows the recorded outcome', async () => {
    const fetchMock = fetchResolving({
      customerStatus: 'ACCEPT',
      createdAt: '2026-09-16T12:00:00Z',
      quoteStatus: 'ACCEPTED',
    });
    vi.stubGlobal('fetch', fetchMock);

    const view = renderActions({ token: 'tok-abc' });

    fireEvent.click(view.getByRole('button', { name: copy.acceptLabel }));
    expect(view.getByText(copy.acceptTitle)).toBeTruthy();
    expect(view.getByText(copy.acceptDescription)).toBeTruthy();

    fireEvent.click(view.getByRole('button', { name: copy.confirm }));

    await waitFor(() => expect(view.getByText(copy.resultTitle)).toBeTruthy());
    expect(view.getByText(copy.resultAccept)).toBeTruthy();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/public-quote-action');
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      token: 'tok-abc',
      type: 'ACCEPT',
    });
    vi.unstubAllGlobals();
  });

  it('requires a message before a change request can go out', async () => {
    const fetchMock = fetchResolving({
      customerStatus: 'REQUEST_CHANGE',
      createdAt: '2026-09-16T12:00:00Z',
      quoteStatus: 'CHANGE_REQUESTED',
    });
    vi.stubGlobal('fetch', fetchMock);

    const view = renderActions({ token: 'tok-abc' });

    fireEvent.click(view.getByRole('button', { name: copy.requestChangesLabel }));
    const confirm = view.getByRole('button', { name: copy.confirm });
    fireEvent.click(confirm);

    expect(view.getByText(copy.messageRequired)).toBeTruthy();
    expect(fetchMock).not.toHaveBeenCalled();

    const textarea = view.getByRole('textbox', { name: copy.messageLabel });
    fireEvent.change(textarea, { target: { value: 'Cambiar el cemento por 25kg' } });
    fireEvent.click(confirm);

    await waitFor(() => expect(view.getByText(copy.resultTitle)).toBeTruthy());
    expect(view.getByText(copy.resultRequestChange)).toBeTruthy();
    expect(JSON.parse((fetchMock.mock.calls[0]![1] as RequestInit).body as string)).toEqual({
      token: 'tok-abc',
      type: 'REQUEST_CHANGE',
      message: 'Cambiar el cemento por 25kg',
    });
    vi.unstubAllGlobals();
  });

  it('reports the send refusal inline and lets the customer retry', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 502 }));
    const view = renderActions({ token: 'tok-abc' });

    fireEvent.click(view.getByRole('button', { name: copy.rejectLabel }));
    fireEvent.click(view.getByRole('button', { name: copy.confirm }));

    await waitFor(() => expect(view.getByText(copy.errorSubmit)).toBeTruthy());
    expect(view.getByText(copy.rejectTitle)).toBeTruthy();
    vi.unstubAllGlobals();
  });

  it('demoes the outcome on the preview route without a round trip', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const view = renderActions({});

    fireEvent.click(view.getByRole('button', { name: copy.acceptLabel }));
    fireEvent.click(view.getByRole('button', { name: copy.confirm }));

    await waitFor(() => expect(view.getByText(copy.resultTitle)).toBeTruthy());
    expect(view.getByText(copy.resultAccept)).toBeTruthy();
    expect(fetchMock).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });

  it('shows the saved answer instead of the decision buttons once answered', () => {
    const view = renderActions({ token: 'tok-abc', customerStatus: 'REQUEST_CHANGE' });

    expect(view.getByText(copy.respondedTitle)).toBeTruthy();
    expect(view.getByText(copy.respondedRequestChange)).toBeTruthy();
    expect(view.queryByRole('button', { name: copy.acceptLabel })).toBeNull();
  });
});
