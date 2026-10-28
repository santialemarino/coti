import { forwardToApi } from '@/lib/api/upstream';

export async function PATCH(
  request: Request,
  {
    params,
  }: {
    params: Promise<{ quoteId: string; itemId: string; alternativeId: string }>;
  },
) {
  const { quoteId, itemId, alternativeId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/items/${itemId}/alternatives/${alternativeId}`,
    method: 'PATCH',
    body: await request.text(),
  });
}
