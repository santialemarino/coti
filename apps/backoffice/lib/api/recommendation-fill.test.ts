import { afterEach, describe, expect, it, vi } from 'vitest';

import { fillQuoteRecommendations } from '@/lib/api/rfqs-client';

afterEach(() => vi.unstubAllGlobals());

describe('fillQuoteRecommendations', () => {
  it('posts to the quote branch and pairs each added alternative with its item', async () => {
    const alternative = { id: 'alt', product_id: 'product', recommendation_type: 'ECONOMY' };
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ items: [{ item_id: 'item', alternative }] }), {
        status: 200,
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    expect(await fillQuoteRecommendations('quote', 'branch')).toEqual([
      { itemId: 'item', alternative },
    ]);
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/quotes/quote/recommendations/fill',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-Branch-Id': 'branch' }),
        body: '{}',
      }),
    );
  });

  it('throws the API refusal', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify({ error: 'conflict', code: 'CONFLICT' }), { status: 409 }),
        ),
    );
    await expect(fillQuoteRecommendations('quote', 'branch')).rejects.toMatchObject({
      code: 'CONFLICT',
    });
  });
});
