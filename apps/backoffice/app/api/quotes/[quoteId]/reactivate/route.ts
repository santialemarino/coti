import { forwardToApi } from '@/lib/api/upstream';

// BFF proxy for reopening an accepted or rejected quote.
export async function POST(request: Request, { params }: { params: Promise<{ quoteId: string }> }) {
  const { quoteId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/reactivate`,
    method: 'POST',
    body: await request.text(),
  });
}
