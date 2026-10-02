import { NextResponse, type NextRequest } from 'next/server';

import { ROUTES } from '@/config/routes';
import { BRANCH_COOKIE } from '@/lib/auth/tokens';
import { redirectTo } from '@/lib/utils/redirect';

// The header the client router sends when it fetches a route's flight data instead of loading it.
const FLIGHT_REQUEST_HEADER = 'rsc';

/*
 * Where a stale branch cookie is dropped, because neither a layout nor a page can write cookies. A
 * client navigation arrives as a flight fetch, and following it home would keep the shell showing the
 * dropped branch; answering it with no flight makes the router load the page in full instead.
 */
export async function GET(request: NextRequest) {
  const response = request.headers.has(FLIGHT_REQUEST_HEADER)
    ? new NextResponse(null, { status: 204 })
    : redirectTo(ROUTES.home);
  response.cookies.delete(BRANCH_COOKIE);
  return response;
}
