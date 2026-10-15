import { NextRequest } from 'next/server';
import { describe, expect, it, vi } from 'vitest';

import { PublicQuoteActionError } from '@/lib/api/public-quotes';
import { POST } from './route';

vi.mock('@/lib/api/public-quotes', async (original) => ({
  ...(await original<typeof import('@/lib/api/public-quotes')>()),
  submitPublicQuoteAction: vi.fn(),
}));

const { submitPublicQuoteAction } = await import('@/lib/api/public-quotes');

function answer() {
  return new NextRequest('http://localhost/api/public-quote-action', {
    method: 'POST',
    body: JSON.stringify({ token: 'tok-1', type: 'ACCEPT' }),
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
});
