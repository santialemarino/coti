import { forwardToApi } from '@/lib/api/upstream';

export async function POST(
  request: Request,
  { params }: { params: Promise<{ messageId: string }> },
) {
  const { messageId } = await params;
  return forwardToApi(request, {
    path: `/v1/inbound-messages/${encodeURIComponent(messageId)}/order`,
    method: 'POST',
  });
}
