import { forwardToApi } from '@/lib/api/upstream';

const QUEUE_PATH = '/v1/rfqs';
const ARCHIVED_PARAM = 'include_archived';

/*
 * BFF proxy for GET /v1/rfqs, which the queue re-reads from the browser: the column lives in a
 * layout that a navigation never re-renders, so the server read alone would go stale. It scopes
 * exactly as that server read does, or a re-read could swap an account-wide list for one branch's.
 * Archived orders come down only when the caller asks for them.
 */
export async function GET(request: Request) {
  const archived = new URL(request.url).searchParams.get(ARCHIVED_PARAM) === 'true';
  return forwardToApi(request, {
    path: archived ? `${QUEUE_PATH}?${ARCHIVED_PARAM}=true` : QUEUE_PATH,
    method: 'GET',
    fallbackToOnlyBranch: false,
  });
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
