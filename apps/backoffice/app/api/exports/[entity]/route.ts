import { forwardToApi } from '@/lib/api/upstream';

const FORWARDED_PARAMS = ['format', 'branch_id', 'from', 'to'] as const;

export async function GET(request: Request, { params }: { params: Promise<{ entity: string }> }) {
  const { entity } = await params;
  const query = new URL(request.url).searchParams;
  const forwarded = new URLSearchParams();
  FORWARDED_PARAMS.forEach((key) => {
    const value = query.get(key);
    if (value) forwarded.set(key, value);
  });
  return forwardToApi(request, {
    path: `/v1/exports/${encodeURIComponent(entity)}?${forwarded.toString()}`,
    method: 'GET',
    fallbackToOnlyBranch: false,
  });
}
