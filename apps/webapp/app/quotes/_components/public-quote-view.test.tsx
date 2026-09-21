import { createElement } from 'react';
import { render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import {
  EXPIRES_AT,
  makePublicQuoteSend,
  makeQuotePayload,
} from '@/lib/api/public-quotes.fixtures';
import { getFormatters } from '@/lib/i18n/formatters-server';
import { PublicQuoteView } from './public-quote-view';

vi.mock('next-intl/server', async () => import('@/lib/i18n/server-intl.mock'));

/*
 * The shared view is composed of the same async pieces the old route carried, which the client
 * renderer cannot await — its children stay stubbed and their own specs cover them. This spec keeps
 * how the view composes: greeting, block order and the expiry answer for a non-ACTIVE send.
 */
vi.mock('@/app/quotes/[token]/_components/quote-header', () => ({
  QuoteHeader: ({ reference }: { reference: string }) => (
    <p data-testid="quote-header">{reference}</p>
  ),
}));
vi.mock('@/app/quotes/[token]/_components/quote-items', () => ({
  QuoteItems: () => <p data-testid="quote-items" />,
}));
vi.mock('@/app/quotes/[token]/_components/quote-info', () => ({
  QuoteInfo: () => <p data-testid="quote-info" />,
}));
vi.mock('@/app/quotes/[token]/_components/quote-actions', () => ({
  QuoteActions: ({ customerStatus }: { customerStatus?: string }) =>
    customerStatus ? (
      <p data-testid="quote-actions">{customerStatus}</p>
    ) : (
      <p data-testid="quote-actions" />
    ),
}));
vi.mock('@/components/brand', () => ({
  Brand: ({ label }: { label?: string }) =>
    createElement('img', { alt: label, src: '/brand.mock.png' }),
}));

describe('PublicQuoteView', () => {
  it('renders the quote blocks and the greeting of an active send', async () => {
    const view = render(await PublicQuoteView({ send: makePublicQuoteSend() }));

    expect(view.getByText('Hola, Obra Norte.')).toBeTruthy();
    expect(view.getByTestId('quote-header').textContent).toBe('COT-000042');
    expect(view.getByTestId('quote-items')).toBeTruthy();
    expect(view.getByTestId('quote-info')).toBeTruthy();
    // The actions stay open for a send the customer has not answered yet.
    expect(view.getByTestId('quote-actions').textContent).toBe('');
    expect(view.getByAltText('Coti')).toBeTruthy();
  });

  it('forwards a send that was already answered to the actions block', async () => {
    const view = render(
      await PublicQuoteView({
        send: makePublicQuoteSend({ customerStatus: 'ACCEPT' }),
        token: 'tok-abc',
      }),
    );

    expect(view.getByTestId('quote-actions').textContent).toBe('ACCEPT');
  });

  it('greets without a name when the customer is anonymous', async () => {
    const send = makePublicQuoteSend({
      quote: { ...makeQuotePayload(), customerName: undefined },
    });

    const view = render(await PublicQuoteView({ send }));

    expect(view.getByText('Hola.')).toBeTruthy();
  });

  it('shows the expiry screen for an expired send, never a quote', async () => {
    const send = makePublicQuoteSend({ status: 'EXPIRED', quote: undefined, pdfUrl: undefined });
    const fmt = await getFormatters();

    const view = render(await PublicQuoteView({ send }));

    expect(view.getByText('Esta cotización venció')).toBeTruthy();
    expect(
      view.getByText(
        `El enlace dejó de estar disponible el ${fmt.date(EXPIRES_AT)}. Escribile al corralón y pedile una cotización nueva.`,
      ),
    ).toBeTruthy();
    expect(view.queryByTestId('quote-header')).toBeNull();
    expect(view.queryByTestId('quote-items')).toBeNull();
    expect(view.queryByTestId('quote-info')).toBeNull();
    expect(view.queryByTestId('quote-actions')).toBeNull();
  });
});
