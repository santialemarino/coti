import { forwardToApi } from '@/lib/api/upstream';

/*
 * BFF proxy for PATCH /v1/quotes/:quoteId/discounts/:discountId. Patches a discount application
 * on a mutable version; the backend recomputes the version total when a write lands.
 */
export async function PATCH(
  request: Request,
  { params }: { params: Promise<{ quoteId: string; discountId: string }> },
) {
  const { quoteId, discountId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/discounts/${discountId}`,
    method: 'PATCH',
    body: await request.text(),
  });
}

/*
 * BFF proxy for DELETE /v1/quotes/:quoteId/discounts/:discountId. Removes a MANUAL_SELLER
 * discount for good; the backend refuses deleting an engine-applied one.
 */
export async function DELETE(
  request: Request,
  { params }: { params: Promise<{ quoteId: string; discountId: string }> },
) {
  const { quoteId, discountId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/discounts/${discountId}`,
    method: 'DELETE',
  });
}
