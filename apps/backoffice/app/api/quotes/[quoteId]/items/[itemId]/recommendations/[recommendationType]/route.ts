import { forwardToApi } from '@/lib/api/upstream';

interface RouteContext {
  params: Promise<{ quoteId: string; itemId: string; recommendationType: string }>;
}

export async function PUT(request: Request, { params }: RouteContext) {
  const { quoteId, itemId, recommendationType } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/items/${itemId}/recommendations/${recommendationType}`,
    method: 'PUT',
    body: await request.text(),
  });
}
