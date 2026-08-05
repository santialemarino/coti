import { NextResponse, type NextRequest } from 'next/server';

import {
  LOGIN_ROUTE,
  NEXT_PARAM,
  PUBLIC_ROUTES,
  ROUTES,
  SIGNED_OUT_ONLY_ROUTES,
} from '@/config/routes';
import {
  ACCESS_COOKIE,
  BRANCH_COOKIE,
  forwardedClientAddress,
  needsRenewal,
  REFRESH_COOKIE,
  REMEMBER_COOKIE,
  requestRefresh,
  sessionCookieOptions,
} from '@/lib/auth/tokens';

/*
 * The gate, and the only place a session is renewed: of the three contexts Next
 * allows a cookie write from, middleware is the one that runs before the page
 * renders. Renewing here is what lets a server component read a live token without
 * ever handling expiry itself.
 *
 * It decides reachability, not authorization. It knows whether a token exists and
 * whether it has expired; whether the session behind it is still good is the API's
 * answer, which the protected layout asks for on every render.
 */
export async function middleware(request: NextRequest) {
  const { pathname, search } = request.nextUrl;
  const accessToken = request.cookies.get(ACCESS_COOKIE)?.value;
  const refreshToken = request.cookies.get(REFRESH_COOKIE)?.value;
  const remembered = request.cookies.get(REMEMBER_COOKIE)?.value === '1';

  if (PUBLIC_ROUTES.includes(pathname)) {
    if (SIGNED_OUT_ONLY_ROUTES.includes(pathname) && accessToken && !needsRenewal(accessToken)) {
      return NextResponse.redirect(new URL(ROUTES.home, request.url));
    }
    return NextResponse.next();
  }

  if (!needsRenewal(accessToken)) return NextResponse.next();

  /*
   * A prefetch renders nothing the user is looking at, so it does not get to spend a
   * refresh token. Letting it would put several renewals inside the same skew window,
   * and the API reads a replayed refresh token past its grace window as theft.
   */
  if (request.headers.get('next-router-prefetch') === '1') return NextResponse.next();

  // An expired access token is not an expired session, and renewing it is what
  // keeps the user from being thrown out mid-task.
  if (refreshToken) {
    const renewed = await requestRefresh(refreshToken, forwardedClientAddress(request.headers));
    if (renewed.ok && renewed.tokens) {
      // Onto the request too, so the render this triggers sees the new token rather
      // than waiting for the next round trip.
      request.cookies.set(ACCESS_COOKIE, renewed.tokens.accessToken);
      request.cookies.set(REFRESH_COOKIE, renewed.tokens.refreshToken);
      const response = NextResponse.next({ request });
      const options = sessionCookieOptions(remembered);
      response.cookies.set(ACCESS_COOKIE, renewed.tokens.accessToken, options);
      response.cookies.set(REFRESH_COOKIE, renewed.tokens.refreshToken, options);
      return response;
    }
    if (renewed.status === 0) return NextResponse.next();
  }

  return redirectToLogin(request, pathname + search);
}

function redirectToLogin(request: NextRequest, from: string) {
  const target = new URL(LOGIN_ROUTE, request.url);
  if (from !== ROUTES.home) target.searchParams.set(NEXT_PARAM, from);

  const response = NextResponse.redirect(target);
  // Clearing is what stops the bounce: a surviving unexpired token would send the
  // login screen straight back to a page that rejects it.
  response.cookies.delete(ACCESS_COOKIE);
  response.cookies.delete(REFRESH_COOKIE);
  response.cookies.delete(REMEMBER_COOKIE);
  response.cookies.delete(BRANCH_COOKIE);
  return response;
}

export const config = {
  // Anchored on whole segments, so a future route merely starting with "icons" or
  // ending in ".svg" cannot slip past the gate. The image optimiser is matched with
  // and without a trailing slash: its own URL is exactly /_next/image plus a query
  // string, so a slash-only exclusion never fires and every optimised image would
  // be sent to the login screen instead.
  matcher: ['/((?!_next/static/|_next/image$|_next/image/|favicon\\.ico$|icons/|brand/).*)'],
};
