import { forwardToApi } from '@/lib/api/upstream';

export async function PUT(
  request: Request,
  { params }: { params: Promise<{ clientId: string; tagId: string }> },
) {
  const { clientId, tagId } = await params;
  return forwardToApi(request, {
    path: `/v1/clients/${encodeURIComponent(clientId)}/tag-exclusions/${encodeURIComponent(tagId)}`,
    method: 'PUT',
    body: await request.text(),
  });
}
