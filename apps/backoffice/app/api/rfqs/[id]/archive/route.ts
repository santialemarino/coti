import { forwardToApi } from '@/lib/api/upstream';

export async function PUT(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return forwardToApi(request, {
    path: '/v1/rfqs/' + id + '/archive',
    method: 'PUT',
    body: await request.text(),
  });
}
