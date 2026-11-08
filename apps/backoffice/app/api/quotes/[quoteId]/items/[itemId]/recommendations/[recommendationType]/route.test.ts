import { beforeEach, describe, expect, it, vi } from 'vitest';

const fetchMock = vi.fn();

vi.mock('next/headers', () => ({
  cookies: vi.fn(async () => ({ get: vi.fn(() => ({ value: 'access-token' })) })),
}));

const { PUT } =
  await import('@/app/api/quotes/[quoteId]/items/[itemId]/recommendations/[recommendationType]/route');

const BRANCH_ID = '11111111-1111-4111-8111-111111111111';
const QUOTE_ID = '22222222-2222-4222-8222-222222222222';
const ITEM_ID = '33333333-3333-4333-8333-333333333333';

describe('quote recommendation proxy', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal('fetch', fetchMock.mockResolvedValue(new Response('{}', { status: 200 })));
  });

  it('forwards a seller-defined premium recommendation with its branch scope', async () => {
    const response = await PUT(
      new Request('http://localhost/api/quotes/quote/items/item/recommendations/PREMIUM', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', 'X-Branch-Id': BRANCH_ID },
        body: JSON.stringify({ product_id: 'product-id', save_to_catalog: true }),
      }),
      {
        params: Promise.resolve({
          quoteId: QUOTE_ID,
          itemId: ITEM_ID,
          recommendationType: 'PREMIUM',
        }),
      },
    );

    expect(response.status).toBe(200);
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(url).toContain(`/v1/quotes/${QUOTE_ID}/items/${ITEM_ID}/recommendations/PREMIUM`);
    expect(init?.method).toBe('PUT');
    expect(new Headers(init?.headers).get('X-Branch-Id')).toBe(BRANCH_ID);
    expect(init?.body).toBe(JSON.stringify({ product_id: 'product-id', save_to_catalog: true }));
  });
});
