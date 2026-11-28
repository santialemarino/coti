import { forwardToApi } from '@/lib/api/upstream';

// BFF proxy for GET /v1/inbound-messages/non-orders, the branch's messages that opened no order.
export async function GET(request: Request) {
  return forwardToApi(request, { path: '/v1/inbound-messages/non-orders', method: 'GET' });
}
