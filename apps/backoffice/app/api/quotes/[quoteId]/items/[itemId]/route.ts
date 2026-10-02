import { forwardToApi } from '@/lib/api/upstream';

/*
 * BFF proxy for PATCH /v1/quotes/:quoteId/items/:itemId. Updates a draft quote item.
 */
export async function PATCH(
  request: Request,
  { params }: { params: Promise<{ quoteId: string; itemId: string }> },
) {
  const { quoteId, itemId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/items/${itemId}`,
    method: 'PATCH',
    body: await request.text(),
  });
}

/*
 * BFF proxy for DELETE /v1/quotes/:quoteId/items/:itemId. Deletes a draft quote item.
 */
export async function DELETE(
  request: Request,
  { params }: { params: Promise<{ quoteId: string; itemId: string }> },
) {
  const { quoteId, itemId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/items/${itemId}`,
    method: 'DELETE',
  });
}
