import { createElement } from 'react';
import { render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import {
  EXPIRES_AT,
  makePublicQuoteSend,
  makeQuotePayload,
} from '@/lib/api/public-quotes.fixtures';
import { getFormatters } from '@/lib/i18n/formatters-server';
import messages from '@/translations/es.json';
import { PublicQuoteView } from './public-quote-view';

vi.mock('next-intl/server', async () => import('@/lib/i18n/server-intl.mock'));

/*
 * The shared view is composed of the same async pieces the old route carried, which the client
 * renderer cannot await — its children stay stubbed and their own specs cover them. This spec keeps
 * how the view composes: greeting, block order and the expiry answer for a non-ACTIVE send.
 */
vi.mock('@/app/quotes/[token]/_components/quote-header', () => ({
  QuoteHeader: ({ reference, versionNumber }: { reference: string; versionNumber?: number }) => (
    <p data-testid="quote-header" data-version={versionNumber ?? 'none'}>
      {reference}
    </p>
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
vi.mock('@/app/quotes/[token]/_components/quote-contact', () => ({
  QuoteContact: ({ branch }: { branch: { name: string } }) => (
    <p data-testid="quote-contact">{branch.name}</p>
  ),
}));
vi.mock('@/components/brand', () => ({
  Brand: ({ label }: { label?: string }) =>
    createElement('img', { alt: label, src: '/brand.mock.png' }),
}));

describe('PublicQuoteView', () => {
  // TC-12: once the customer accepted with alternatives, the view is that accepted version, and the
  // seller's version number means nothing to them.
  it('drops the seller version stamp on the version the customer accepted', async () => {
    const sent = render(await PublicQuoteView({ send: makePublicQuoteSend() }));
    expect(sent.getByTestId('quote-header').dataset.version).not.toBe('none');
    sent.unmount();
    const accepted = render(
      await PublicQuoteView({ send: { ...makePublicQuoteSend(), acceptedVersion: true } }),
    );
    expect(accepted.getByTestId('quote-header').dataset.version).toBe('none');
  });

  it('renders the quote blocks and the greeting of an active send', async () => {
    const view = render(await PublicQuoteView({ send: makePublicQuoteSend() }));

    expect(view.getByText('Hola, Obra Norte.')).toBeTruthy();
    expect(view.getByTestId('quote-header').textContent).toBe('COT-000042');
    expect(view.getByTestId('quote-items')).toBeTruthy();
    expect(view.getByTestId('quote-info')).toBeTruthy();
    // The actions stay open for a send the customer has not answered yet.
    expect(view.getByTestId('quote-actions').textContent).toBe('');
    // The quote is the corralón's: no Coti mark beside it, and a way to write back to the branch.
    expect(view.queryByAltText('Coti')).toBeNull();
    expect(view.getByTestId('quote-contact').textContent).toBe('Casa central');
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
    expect(view.queryByAltText('Coti')).toBeNull();
  });

  it('says a withdrawn quote is no longer available, never showing it', async () => {
    const send = makePublicQuoteSend({
      status: 'UNAVAILABLE',
      quote: undefined,
      pdfUrl: undefined,
    });

    const view = render(await PublicQuoteView({ send }));

    expect(view.getByText(messages.quote.unavailableTitle)).toBeTruthy();
    expect(view.getByText(messages.quote.unavailableDescription)).toBeTruthy();
    expect(view.queryByText(messages.quote.expiredTitle)).toBeNull();
    expect(view.queryByTestId('quote-actions')).toBeNull();
  });
});
