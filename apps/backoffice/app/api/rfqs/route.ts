import { forwardToApi } from '@/lib/api/upstream';

// Archived orders come down with the rest; the screens filter them out themselves.
const QUEUE_PATH = '/v1/rfqs?include_archived=true';

/*
 * BFF proxy for GET /v1/rfqs, which the queue re-reads from the browser: the column lives in a
 * layout that a navigation never re-renders, so the server read alone would go stale.
 */
export async function GET(request: Request) {
  return forwardToApi(request, { path: QUEUE_PATH, method: 'GET' });
}

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
