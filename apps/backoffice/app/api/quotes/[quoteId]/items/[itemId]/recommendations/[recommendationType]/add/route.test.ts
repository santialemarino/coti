import { beforeEach, describe, expect, it, vi } from 'vitest';

const fetchMock = vi.fn();
vi.mock('next/headers', () => ({
  cookies: vi.fn(async () => ({ get: vi.fn(() => ({ value: 'access-token' })) })),
}));

const { POST } =
  await import('@/app/api/quotes/[quoteId]/items/[itemId]/recommendations/[recommendationType]/add/route');

describe('recommendation add proxy', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal('fetch', fetchMock.mockResolvedValue(new Response('{}', { status: 200 })));
  });

  it('forwards the branch, item, type and product to the vacancy-guarded endpoint', async () => {
    const response = await POST(
      new Request('http://localhost/api/quotes/quote/items/item/recommendations/ECONOMY/add', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-Branch-Id': 'branch' },
        body: JSON.stringify({ product_id: 'product' }),
      }),
      {
        params: Promise.resolve({
          quoteId: 'quote',
          itemId: 'item',
          recommendationType: 'ECONOMY',
        }),
      },
    );
    expect(response.status).toBe(200);
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(url).toContain('/v1/quotes/quote/items/item/recommendations/ECONOMY/add');
    expect(init?.method).toBe('POST');
    expect(new Headers(init?.headers).get('X-Branch-Id')).toBe('branch');
    expect(init?.body).toBe(JSON.stringify({ product_id: 'product' }));
  });
});
