import { forwardToApi } from '@/lib/api/upstream';

// BFF proxy for GET /v1/quotes/:quoteId/delivery-preset, keeping the mode and client query.
export async function GET(request: Request, { params }: { params: Promise<{ quoteId: string }> }) {
  const { quoteId } = await params;
  const { search } = new URL(request.url);
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/delivery-preset${search}`,
    method: 'GET',
  });
}
