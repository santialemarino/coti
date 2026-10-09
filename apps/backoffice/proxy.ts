import { NextResponse, type NextRequest } from 'next/server';

import {
  isProtectedPath,
  LOCKED_REASON,
  LOGIN_ROUTE,
  NEXT_PARAM,
  REASON_PARAM,
  ROUTES,
  safeNextPath,
  SESSION_CLEARING_ROUTES,
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

const SESSION_COOKIES = [ACCESS_COOKIE, REFRESH_COOKIE, REMEMBER_COOKIE, BRANCH_COOKIE] as const;

/*
 * The gate, and the only place a session is renewed: of the three contexts Next
 * allows a cookie write from, this is the one that runs before the page renders.
 * Renewing here is what lets a server component read a live token without ever
 * handling expiry itself.
 *
 * It decides reachability, not authorization. It knows whether a token exists and
 * whether it has expired; whether the session behind it is still good is the API's
 * answer, which the protected layout asks for on every render.
 */
export async function proxy(request: NextRequest) {
  const { pathname, search } = request.nextUrl;
  const accessToken = request.cookies.get(ACCESS_COOKIE)?.value;
  const refreshToken = request.cookies.get(REFRESH_COOKIE)?.value;
  const remembered = request.cookies.get(REMEMBER_COOKIE)?.value === '1';
  const guarded = isProtectedPath(pathname);
  const signedOutOnly = SIGNED_OUT_ONLY_ROUTES.includes(pathname);
  // A signed-in caller on the login screen goes where `next` asked, so a second click on a link works.
  const onward = new URL(safeNextPath(request.nextUrl.searchParams.get(NEXT_PARAM)), request.url);

  // A visitor: the public site and the 404 are theirs, and anything guarded asks them to log in.
  if (!accessToken && !refreshToken) {
    return guarded ? redirectToLogin(request, pathname + search) : NextResponse.next();
  }

  if (SESSION_CLEARING_ROUTES.includes(pathname)) return NextResponse.next();

  if (!needsRenewal(accessToken)) {
    return signedOutOnly ? NextResponse.redirect(onward) : NextResponse.next();
  }

  /*
   * A prefetch renders nothing the user is looking at, so it does not get to spend a
   * refresh token. Letting it would put several renewals inside the same skew window,
   * and the API reads a replayed refresh token past its grace window as theft.
   */
  if (request.headers.get('next-router-prefetch') === '1') return NextResponse.next();

  /*
   * An expired access token is not an expired session. Renewing it on every page, the public ones
   * included, settles the question once per request: past this point, still holding the cookies
   * means holding a live session, which is all the header, the landing and the 404 need to read.
   */
  const renewed = refreshToken
    ? await requestRefresh(refreshToken, forwardedClientAddress(request.headers))
    : null;
  if (renewed?.ok && renewed.tokens) {
    // Onto the request too, so the render this triggers sees the new token rather
    // than waiting for the next round trip.
    request.cookies.set(ACCESS_COOKIE, renewed.tokens.accessToken);
    request.cookies.set(REFRESH_COOKIE, renewed.tokens.refreshToken);
    const response = signedOutOnly ? NextResponse.redirect(onward) : NextResponse.next({ request });
    const options = sessionCookieOptions(remembered);
    response.cookies.set(ACCESS_COOKIE, renewed.tokens.accessToken, options);
    response.cookies.set(REFRESH_COOKIE, renewed.tokens.refreshToken, options);
    return response;
  }
  // The API is unreachable: the cookies survive and the next request retries.
  if (renewed?.status === 0) return NextResponse.next();

  if (guarded) {
    const reason = renewed?.code === 'ACCOUNT_LOCKED' ? LOCKED_REASON : undefined;
    return redirectToLogin(request, pathname + search, reason);
  }
  // On a public page or a 404 the session just ends, and the page renders for a visitor.
  return endSession(request);
}

function redirectToLogin(request: NextRequest, from: string, reason?: string) {
  const target = new URL(LOGIN_ROUTE, request.url);
  if (from !== ROUTES.home) target.searchParams.set(NEXT_PARAM, from);
  if (reason) target.searchParams.set(REASON_PARAM, reason);

  const response = NextResponse.redirect(target);
  // Clearing is what stops the bounce: a surviving unexpired token would send the
  // login screen straight back to a page that rejects it.
  SESSION_COOKIES.forEach((name) => response.cookies.delete(name));
  return response;
}

function endSession(request: NextRequest) {
  SESSION_COOKIES.forEach((name) => request.cookies.delete(name));
  const response = NextResponse.next({ request });
  SESSION_COOKIES.forEach((name) => response.cookies.delete(name));
  return response;
}

export const config = {
  // Anchored on whole segments, so a future route merely starting with "icons" or
  // ending in ".svg" cannot slip past the gate. The image optimiser is matched with
  // and without a trailing slash: its own URL is exactly /_next/image plus a query
  // string, so a slash-only exclusion never fires and every optimised image would
  // be sent to the login screen instead.
  matcher: [
    '/((?!_next/static/|_next/image$|_next/image/|favicon\\.ico$|icons/|brand/|icon\\.png$|apple-icon\\.png$|manifest\\.webmanifest$|opengraph-image$|robots\\.txt$|sitemap\\.xml$).*)',
  ],
};
