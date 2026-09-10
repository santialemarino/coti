import { forwardToApi } from '@/lib/api/upstream';

/*
 * BFF proxy for POST /v1/rfqs/:rfqId/assign. Claims an unassigned order for the signed-in
 * seller; the status is untouched, only the owner is stamped.
 */
export async function POST(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return forwardToApi(request, { path: `/v1/rfqs/${id}/assign`, method: 'POST' });
}
