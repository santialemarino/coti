import { forwardToApi } from '@/lib/api/upstream';

interface RouteContext {
  params: Promise<{ quoteId: string }>;
}

export async function POST(request: Request, { params }: RouteContext) {
  const { quoteId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/recommendations/search`,
    method: 'POST',
    body: await request.text(),
  });
}
