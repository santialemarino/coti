import { forwardToApi } from '@/lib/api/upstream';

export async function GET(
  request: Request,
  { params }: { params: Promise<{ invoiceId: string }> },
) {
  const { invoiceId } = await params;
  const headers = new Headers(request.headers);
  const branch = new URL(request.url).searchParams.get('branch');
  if (branch) headers.set('X-Branch-Id', branch);
  return forwardToApi(new Request(request.url, { headers }), {
    path: `/v1/invoices/${encodeURIComponent(invoiceId)}/pdf`,
    method: 'GET',
  });
}
