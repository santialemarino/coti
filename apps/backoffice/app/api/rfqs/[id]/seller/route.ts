import { forwardToApi } from '@/lib/api/upstream';

/*
 * BFF proxy for PUT /v1/rfqs/:rfqId/seller. Admin steering: the body names the seller to put on
 * the order, or a null seller_id to clear it. The API is admin-only here and also checks the
 * named seller serves the order's own branch.
 */
export async function PUT(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return forwardToApi(request, {
    path: `/v1/rfqs/${id}/seller`,
    method: 'PUT',
    body: await request.text(),
  });
}
