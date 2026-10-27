import { forwardToApi } from '@/lib/api/upstream';

export async function POST(
  request: Request,
  { params }: { params: Promise<{ clientId: string }> },
) {
  const { clientId } = await params;
  return forwardToApi(request, {
    path: `/v1/clients/${encodeURIComponent(clientId)}/registration-link`,
    method: 'POST',
    body: await request.text(),
  });
}
