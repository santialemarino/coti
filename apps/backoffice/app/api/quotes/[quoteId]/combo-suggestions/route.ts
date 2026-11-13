import { forwardToApi } from '@/lib/api/upstream';

// BFF proxy for GET /v1/quotes/:quoteId/combo-suggestions.
export async function GET(request: Request, { params }: { params: Promise<{ quoteId: string }> }) {
  const { quoteId } = await params;
  return forwardToApi(request, { path: `/v1/quotes/${quoteId}/combo-suggestions`, method: 'GET' });
}
