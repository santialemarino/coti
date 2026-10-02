import { forwardToApi } from '@/lib/api/upstream';

/*
 * BFF proxy for the archive pair. DELETE is the unarchive: putting an order back in the queue is
 * the removal of the flag, and the API spells that as its own endpoint.
 */
export async function POST(request: Request, { params }: { params: Promise<{ quoteId: string }> }) {
  const { quoteId } = await params;
  return forwardToApi(request, { path: `/v1/quotes/${quoteId}/archive`, method: 'POST' });
}

export async function DELETE(
  request: Request,
  { params }: { params: Promise<{ quoteId: string }> },
) {
  const { quoteId } = await params;
  return forwardToApi(request, { path: `/v1/quotes/${quoteId}/unarchive`, method: 'POST' });
}
