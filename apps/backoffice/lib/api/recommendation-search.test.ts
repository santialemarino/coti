import { afterEach, describe, expect, it, vi } from 'vitest';

import { addQuoteRecommendationIfVacant, searchQuoteRecommendations } from '@/lib/api/rfqs-client';

afterEach(() => vi.unstubAllGlobals());

describe('recommendation search', () => {
  it('maps returned proposals and scopes the request to the quote branch', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          items: [
            {
              item_id: 'item',
              product_id: 'product',
              recommendation_type: 'ECONOMY',
              name: 'Cal',
              code: 'CAL',
              unit: 'bolsa',
              unit_price: '10.25',
              currency: 'ARS',
            },
          ],
        }),
        { status: 200 },
      ),
    );
    vi.stubGlobal('fetch', fetchMock);
    expect(await searchQuoteRecommendations('quote', 'branch')).toEqual([
      {
        itemId: 'item',
        productId: 'product',
        recommendationType: 'ECONOMY',
        name: 'Cal',
        code: 'CAL',
        unit: 'bolsa',
        unitPrice: '10.25',
        currency: 'ARS',
      },
    ]);
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/quotes/quote/recommendations/search',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-Branch-Id': 'branch' }),
        body: '{}',
      }),
    );
  });

  it('posts a chosen product to the vacancy-guarded endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('{}', { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    await addQuoteRecommendationIfVacant('quote', 'item', 'PREMIUM', 'branch', 'product');
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/quotes/quote/items/item/recommendations/PREMIUM/add',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ 'X-Branch-Id': 'branch' }),
        body: JSON.stringify({ product_id: 'product' }),
      }),
    );
  });
});
