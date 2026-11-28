import { forwardToApi } from '@/lib/api/upstream';

const ACTIONS = new Set(['confirm', 'undo']);

// BFF proxy for confirming or undoing an acceptance read from the customer's reply.
export async function POST(
  request: Request,
  { params }: { params: Promise<{ quoteId: string; action: string }> },
) {
  const { quoteId, action } = await params;
  if (!ACTIONS.has(action)) return new Response(null, { status: 404 });
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/acceptance/${action}`,
    method: 'POST',
  });
}
