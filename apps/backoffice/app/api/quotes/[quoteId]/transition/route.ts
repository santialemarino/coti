import { forwardToApi } from '@/lib/api/upstream';

// BFF proxy for seller-driven quote closure transitions.
export async function POST(request: Request, { params }: { params: Promise<{ quoteId: string }> }) {
  const { quoteId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/transition`,
    method: 'POST',
    body: await request.text(),
  });
}
