import { forwardToApi } from '@/lib/api/upstream';

export async function GET(request: Request) {
  const query = new URL(request.url).searchParams;
  const params = new URLSearchParams();
  for (const key of ['from', 'to']) {
    const value = query.get(key);
    if (value) params.set(key, value);
  }
  const search = params.toString();
  return forwardToApi(request, {
    path: `/v1/reports/pdf${search ? `?${search}` : ''}`,
    method: 'GET',
    fallbackToOnlyBranch: false,
  });
}
