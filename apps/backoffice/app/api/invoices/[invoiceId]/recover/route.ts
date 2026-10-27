import { forwardToApi } from '@/lib/api/upstream';

export async function POST(
  request: Request,
  { params }: { params: Promise<{ invoiceId: string }> },
) {
  const { invoiceId } = await params;
  return forwardToApi(request, {
    path: `/v1/invoices/${encodeURIComponent(invoiceId)}/recover`,
    method: 'POST',
  });
}
