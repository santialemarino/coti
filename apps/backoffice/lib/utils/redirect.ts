import { NextResponse, type NextRequest } from 'next/server';

const TEMPORARY_REDIRECT = 307;
// The header the client router sends when it fetches a route's flight data instead of loading it.
const FLIGHT_REQUEST_HEADER = 'rsc';

/*
 * The answer for a route handler that writes cookies. An action's redirect is followed on the
 * server, which drops its Set-Cookie, and a client navigation keeps the shell it shows; an empty 204
 * to either makes the router load the route as a document, and that load gets the redirect.
 */
export function redirectDocumentTo(request: NextRequest, target: string): NextResponse {
  if (request.headers.has(FLIGHT_REQUEST_HEADER)) return new NextResponse(null, { status: 204 });
  // Relative: a route handler's request.url names the server's own hostname, not the browser's.
  return new NextResponse(null, { status: TEMPORARY_REDIRECT, headers: { Location: target } });
}
