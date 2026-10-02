import { NextResponse, type NextRequest } from 'next/server';

const TEMPORARY_REDIRECT = 307;
// The header the client router sends when it fetches a route's flight data instead of loading it.
const FLIGHT_REQUEST_HEADER = 'rsc';

/*
 * A route handler's redirect, with a Location relative to the host the browser used. Next builds a
 * route handler's request.url on the server's own hostname (localhost) rather than the request's
 * Host, so an absolute target made from it sends the caller off the app and away from its cookies.
 */
export function redirectTo(target: string): NextResponse {
  return new NextResponse(null, { status: TEMPORARY_REDIRECT, headers: { Location: target } });
}

/*
 * The answer for a route handler that writes cookies. A client navigation or an action redirect
 * arrives as a flight fetch, which would follow a redirect without handing its Set-Cookie to the
 * browser and keep the shell it already shows; an empty 204 makes the router load the route as a
 * document instead, and that load gets the redirect.
 */
export function redirectDocumentTo(request: NextRequest, target: string): NextResponse {
  return request.headers.has(FLIGHT_REQUEST_HEADER)
    ? new NextResponse(null, { status: 204 })
    : redirectTo(target);
}
