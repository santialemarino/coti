import { forwardToApi } from '@/lib/api/upstream';

export async function POST(request: Request, { params }: { params: Promise<{ quoteId: string }> }) {
  const { quoteId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/discard-ai-changes`,
    method: 'POST',
  });
}
