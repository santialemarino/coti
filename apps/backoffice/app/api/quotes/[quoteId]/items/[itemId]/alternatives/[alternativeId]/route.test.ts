import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { cookieJar } from '@repo/vitest-config/cookies';
import { ACCESS_COOKIE } from '@/lib/auth/tokens';

vi.mock('next/headers', () => ({ cookies: vi.fn() }));

const { cookies } = await import('next/headers');
const { PATCH } =
  await import('@/app/api/quotes/[quoteId]/items/[itemId]/alternatives/[alternativeId]/route');

const BRANCH_ID = '11111111-1111-4111-8111-111111111111';
const QUOTE_ID = '22222222-2222-4222-8222-222222222222';
const ITEM_ID = '33333333-3333-4333-8333-333333333333';
const ALTERNATIVE_ID = '44444444-4444-4444-8444-444444444444';

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(cookies).mockResolvedValue(
    cookieJar({ [ACCESS_COOKIE]: 'access-token' }) as unknown as Awaited<
      ReturnType<typeof cookies>
    >,
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('the quote alternative approval proxy', () => {
  it('forwards the seller decision in its quote and branch scope', async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValue(Response.json({ id: ALTERNATIVE_ID }, { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    const response = await PATCH(
      new Request('http://localhost/api/quotes/quote/items/item/alternatives/alternative', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json', 'X-Branch-Id': BRANCH_ID },
        body: JSON.stringify({ approved_by_seller: true }),
      }),
      {
        params: Promise.resolve({
          quoteId: QUOTE_ID,
          itemId: ITEM_ID,
          alternativeId: ALTERNATIVE_ID,
        }),
      },
    );

    expect(response.status).toBe(200);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(url).toContain(`/v1/quotes/${QUOTE_ID}/items/${ITEM_ID}/alternatives/${ALTERNATIVE_ID}`);
    expect(init?.method).toBe('PATCH');
    expect(new Headers(init?.headers).get('X-Branch-Id')).toBe(BRANCH_ID);
    expect(init?.body).toBe(JSON.stringify({ approved_by_seller: true }));
  });
});
