import { forwardToApi } from '@/lib/api/upstream';

/*
 * BFF proxy for POST /v1/quotes/:quoteId/accept-materials, which is what prices a draft.
 */
export async function POST(request: Request, { params }: { params: Promise<{ quoteId: string }> }) {
  const { quoteId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/accept-materials`,
    method: 'POST',
  });
}
