import { forwardToApi } from '@/lib/api/upstream';

/*
 * BFF proxy for GET /v1/channels. The order creator needs the branch's open intake routes
 * before an order exists, so a file-borne one can name the channel it arrived through.
 */
export async function GET(request: Request) {
  return forwardToApi(request, { path: '/v1/channels', method: 'GET' });
}
