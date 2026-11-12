import { forwardToApi } from '@/lib/api/upstream';

// BFF proxy for POST /v1/quotes/:quoteId/combo-suggestions/:comboId/add.
export async function POST(
  request: Request,
  { params }: { params: Promise<{ quoteId: string; comboId: string }> },
) {
  const { quoteId, comboId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/combo-suggestions/${comboId}/add`,
    method: 'POST',
    body: await request.text(),
  });
}
