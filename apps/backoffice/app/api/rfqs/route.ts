import { forwardToApi } from '@/lib/api/upstream';

/*
 * BFF proxy for POST /v1/rfqs, the manual order intake.
 */
export async function POST(request: Request) {
  return forwardToApi(request, {
    path: '/v1/rfqs',
    method: 'POST',
    body: await request.text(),
  });
}
