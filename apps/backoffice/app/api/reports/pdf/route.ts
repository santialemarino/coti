import { forwardToApi } from '@/lib/api/upstream';

export async function GET(request: Request) {
  const query = new URL(request.url).searchParams;
  const params = new URLSearchParams();
  for (const key of [
    'from',
    'to',
    'status',
    'channel',
    'seller_id',
    'client_id',
    'branch_id',
    'tag_id',
  ]) {
    for (const value of query.getAll(key)) {
      if (value) params.append(key, value);
    }
  }
  const search = params.toString();
  return forwardToApi(request, {
    path: `/v1/reports/pdf${search ? `?${search}` : ''}`,
    method: 'GET',
    fallbackToOnlyBranch: false,
  });
}
