import type { NextRequest } from 'next/server';

import { LOCKED_REASON, LOGIN_ROUTE, REASON_PARAM } from '@/config/routes';
import { clearSession } from '@/lib/auth/session';
import { redirectDocumentTo } from '@/lib/utils/redirect';

/*
 * Where a caller goes once the API has ended their session — a bumped epoch, a deactivated
 * user, a revoked token, or a lockout, which carries ?reason=locked. It exists because a layout
 * cannot write cookies: redirecting straight to the login screen with the dead
 * cookies still set would have the proxy bounce the caller back, forever.
 */
export async function GET(request: NextRequest) {
  await clearSession();
  const locked = request.nextUrl.searchParams.get(REASON_PARAM) === LOCKED_REASON;
  return redirectDocumentTo(
    request,
    locked ? `${LOGIN_ROUTE}?${REASON_PARAM}=${LOCKED_REASON}` : LOGIN_ROUTE,
  );
}
