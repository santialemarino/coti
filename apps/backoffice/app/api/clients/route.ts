import { forwardToApi } from '@/lib/api/upstream';

export async function GET(request: Request) {
  return forwardToApi(request, {
    path: '/v1/clients',
    method: 'GET',
  });
}
