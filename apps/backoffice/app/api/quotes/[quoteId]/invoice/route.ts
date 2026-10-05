import { forwardToApi } from '@/lib/api/upstream';

export async function GET(request: Request, { params }: { params: Promise<{ quoteId: string }> }) {
  const { quoteId } = await params;
  return forwardToApi(request, { path: `/v1/quotes/${quoteId}/invoice`, method: 'GET' });
}

export async function POST(request: Request, { params }: { params: Promise<{ quoteId: string }> }) {
  const { quoteId } = await params;
  return forwardToApi(request, { path: `/v1/quotes/${quoteId}/invoice`, method: 'POST' });
}
