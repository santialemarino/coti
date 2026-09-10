import { forwardToApi } from '@/lib/api/upstream';

/*
 * BFF proxy for GET /v1/rfqs/:rfqId. The browser cannot read HttpOnly cookies, so Client
 * Components call this internal route instead of hitting the Go API directly.
 */
export async function GET(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return forwardToApi(request, { path: `/v1/rfqs/${id}`, method: 'GET' });
}
