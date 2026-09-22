import { render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { makePublicQuoteSend } from '@/lib/api/public-quotes.fixtures';
import PublicQuotePage from './page';

const getPublicQuoteByToken = vi.hoisted(() => vi.fn());
const { notFound } = vi.hoisted(() => ({
  notFound: vi.fn(() => {
    throw new Error('NEXT_NOT_FOUND');
  }),
}));

vi.mock('next-intl/server', async () => import('@/lib/i18n/server-intl.mock'));
vi.mock('next/navigation', () => ({ notFound }));
vi.mock('@/lib/api/public-quotes', () => ({ getPublicQuoteByToken }));

/*
 * The route delegates its rendering to PublicQuoteView, which is covered by its own spec and is an
 * async server component the client renderer cannot await — so this spec keeps only what the route
 * decides: which send reaches the view and where an unknown token goes.
 */
vi.mock('@/app/quotes/_components/public-quote-view', () => ({
  PublicQuoteView: ({ send }: { send: { status: string } }) => (
    <p data-testid="public-quote-view">{send.status}</p>
  ),
}));

function params(token = 'tok-1') {
  return { params: Promise.resolve({ token }) };
}

describe('PublicQuotePage', () => {
  it('hands an active send to the shared view', async () => {
    vi.mocked(getPublicQuoteByToken).mockResolvedValue(makePublicQuoteSend({ status: 'ACTIVE' }));

    const view = render(await PublicQuotePage(params()));

    expect(view.getByTestId('public-quote-view').textContent).toBe('ACTIVE');
  });

  it('hands an expired send to the shared view so it answers with the expiry screen', async () => {
    vi.mocked(getPublicQuoteByToken).mockResolvedValue(
      makePublicQuoteSend({ status: 'EXPIRED', quote: undefined, pdfUrl: undefined }),
    );

    const view = render(await PublicQuotePage(params()));

    expect(view.getByTestId('public-quote-view').textContent).toBe('EXPIRED');
  });

  it('answers an unknown token with the 404 the route declares', async () => {
    vi.mocked(getPublicQuoteByToken).mockResolvedValue(null);

    await expect(PublicQuotePage(params('nope'))).rejects.toThrow('NEXT_NOT_FOUND');
    expect(notFound).toHaveBeenCalled();
  });
});
