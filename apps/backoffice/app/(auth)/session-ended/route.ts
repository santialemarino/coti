import type { NextRequest } from 'next/server';

import { LOCKED_REASON, LOGIN_ROUTE, REASON_PARAM } from '@/config/routes';
import { clearSession } from '@/lib/auth/session';
import { redirectTo } from '@/lib/utils/redirect';

/*
 * Where the protected layout sends a caller whose session the API has ended — a
 * bumped epoch, a deactivated user, a revoked token. It exists because a layout
 * cannot write cookies: redirecting straight to the login screen with the dead
 * cookies still set would have the proxy bounce the caller back, forever.
 */
export async function GET(request: NextRequest) {
  await clearSession();
  const locked = request.nextUrl.searchParams.get(REASON_PARAM) === LOCKED_REASON;
  return redirectTo(locked ? `${LOGIN_ROUTE}?${REASON_PARAM}=${LOCKED_REASON}` : LOGIN_ROUTE);
}
