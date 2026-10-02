import { forwardToApi } from '@/lib/api/upstream';

/*
 * BFF proxy for POST /v1/quotes/:quoteId/discounts. Adds a seller-typed discount (fixed amount or
 * percentage, scoped to the total, one item, or a set) to a mutable version; the backend computes
 * the amount and recomputes the version total.
 */
export async function POST(request: Request, { params }: { params: Promise<{ quoteId: string }> }) {
  const { quoteId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/discounts`,
    method: 'POST',
    body: await request.text(),
  });
}
