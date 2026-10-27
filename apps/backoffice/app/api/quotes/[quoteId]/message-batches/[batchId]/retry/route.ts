import { forwardToApi } from '@/lib/api/upstream';

export async function POST(
  request: Request,
  { params }: { params: Promise<{ quoteId: string; batchId: string }> },
) {
  const { quoteId, batchId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/message-batches/${batchId}/retry`,
    method: 'POST',
  });
}
