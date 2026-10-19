import { forwardToApi } from '@/lib/api/upstream';

export async function GET(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return forwardToApi(request, {
    path: `/v1/rfqs/${encodeURIComponent(id)}/attachments`,
    method: 'GET',
    fallbackToOnlyBranch: false,
  });
}
