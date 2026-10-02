import { forwardToApi } from '@/lib/api/upstream';

export async function GET(request: Request, { params }: { params: Promise<{ quoteId: string }> }) {
  const { quoteId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/client-association`,
    method: 'GET',
  });
}

export async function PUT(request: Request, { params }: { params: Promise<{ quoteId: string }> }) {
  const { quoteId } = await params;
  return forwardToApi(request, {
    path: `/v1/quotes/${quoteId}/client-association`,
    method: 'PUT',
    body: await request.text(),
  });
}
