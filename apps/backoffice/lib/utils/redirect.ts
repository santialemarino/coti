import { NextResponse } from 'next/server';

const TEMPORARY_REDIRECT = 307;

/*
 * A route handler's redirect, with a Location relative to the host the browser used. Next builds a
 * route handler's request.url on the server's own hostname (localhost) rather than the request's
 * Host, so an absolute target made from it sends the caller off the app and away from its cookies.
 */
export function redirectTo(target: string): NextResponse {
  return new NextResponse(null, { status: TEMPORARY_REDIRECT, headers: { Location: target } });
}
