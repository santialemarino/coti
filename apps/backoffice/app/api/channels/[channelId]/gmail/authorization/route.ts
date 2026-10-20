import { forwardToApi } from '@/lib/api/upstream';

export async function POST(
  request: Request,
  { params }: { params: Promise<{ channelId: string }> },
) {
  const { channelId } = await params;
  return forwardToApi(request, {
    path: `/v1/channels/${encodeURIComponent(channelId)}/gmail/authorization`,
    method: 'POST',
    fallbackToOnlyBranch: false,
  });
}
