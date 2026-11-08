import { forwardToApi } from '@/lib/api/upstream';

export async function PUT(
  request: Request,
  { params }: { params: Promise<{ suggestionId: string }> },
) {
  const { suggestionId } = await params;
  return forwardToApi(request, {
    path: `/v1/client-contact-suggestions/${encodeURIComponent(suggestionId)}`,
    method: 'PUT',
    body: await request.text(),
    fallbackToOnlyBranch: false,
  });
}
