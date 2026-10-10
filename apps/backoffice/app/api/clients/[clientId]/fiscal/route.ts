import { forwardToApi } from '@/lib/api/upstream';

export async function GET(request: Request, { params }: { params: Promise<{ clientId: string }> }) {
  const { clientId } = await params;
  return forwardToApi(request, {
    path: `/v1/clients/${encodeURIComponent(clientId)}/fiscal`,
    method: 'GET',
  });
}

export async function PUT(request: Request, { params }: { params: Promise<{ clientId: string }> }) {
  const { clientId } = await params;
  return forwardToApi(request, {
    path: `/v1/clients/${encodeURIComponent(clientId)}/fiscal`,
    method: 'PUT',
    body: await request.text(),
  });
}
