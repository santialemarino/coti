import { forwardToApi } from '@/lib/api/upstream';

/*
 * BFF proxy for POST /v1/quotes/:quoteId/items. Adds a new item to a draft quote.
 */
export async function POST(request: Request, { params }: { params: Promise<{ quoteId: string }> }) {
  const { quoteId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/items`,
    method: 'POST',
    body: await request.text(),
  });
}
