import { NextRequest } from 'next/server';
import { describe, expect, it, vi } from 'vitest';

import { PublicQuoteActionError } from '@/lib/api/public-quotes';
import { POST } from './route';

vi.mock('@/lib/auth/customer-session', () => ({
  getCustomerToken: async () => 'customer-session',
}));

vi.mock('@/lib/api/public-quotes', async (original) => ({
  ...(await original<typeof import('@/lib/api/public-quotes')>()),
  submitPublicQuoteAction: vi.fn(),
}));

const { submitPublicQuoteAction } = await import('@/lib/api/public-quotes');

function answer() {
  return new NextRequest('http://localhost/api/public-quote-action', {
    method: 'POST',
    headers: { origin: 'http://localhost' },
    body: JSON.stringify({ token: 'tok-1', type: 'ACCEPT' }),
  });
}

function answerWithSelection() {
  return new NextRequest('http://localhost/api/public-quote-action', {
    method: 'POST',
    headers: { origin: 'http://localhost' },
    body: JSON.stringify({
      token: 'tok-1',
      type: 'ACCEPT',
      selections: [{ item_id: 'item-1', alternative_id: 'alternative-1' }],
    }),
  });
}

describe('POST /api/public-quote-action', () => {
  // The page words an archived quote on its own, so this refusal crosses the proxy by name.
  it('forwards an archived quote by its code', async () => {
    vi.mocked(submitPublicQuoteAction).mockRejectedValue(
      new PublicQuoteActionError(409, 'QUOTE_ARCHIVED'),
    );

    const response = await POST(answer());

    expect(response.status).toBe(409);
    expect(await response.json()).toEqual({ error: 'QUOTE_ARCHIVED' });
  });

  it('keeps every other conflict generic', async () => {
    vi.mocked(submitPublicQuoteAction).mockRejectedValue(
      new PublicQuoteActionError(409, 'QUOTE_NOT_SENT'),
    );

    const response = await POST(answer());

    expect(await response.json()).toEqual({ error: 'CONFLICT' });
  });

  it('forwards a customer-selected alternative in camel case', async () => {
    vi.mocked(submitPublicQuoteAction).mockResolvedValue({
      customerStatus: 'ACCEPT',
      createdAt: '2026-10-28T12:00:00Z',
      quoteStatus: 'ACCEPTED',
      total: '900.00',
    });

    const response = await POST(answerWithSelection());

    expect(response.status).toBe(200);
    expect(submitPublicQuoteAction).toHaveBeenCalledWith('tok-1', {
      type: 'ACCEPT',
      message: undefined,
      selections: [{ itemId: 'item-1', alternativeId: 'alternative-1' }],
    });
  });
});
