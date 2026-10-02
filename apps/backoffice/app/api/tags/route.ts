import { forwardToApi } from '@/lib/api/upstream';

export async function POST(request: Request) {
  return forwardToApi(request, {
    path: '/v1/tags',
    method: 'POST',
    body: await request.text(),
  });
}
