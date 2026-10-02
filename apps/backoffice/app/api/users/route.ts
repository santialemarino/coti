import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';

import { ACCESS_COOKIE, BRANCH_COOKIE } from '@/lib/auth/tokens';
import { API_URL } from '@/lib/config';

/*
 * BFF proxy for GET /v1/sellers. The browser cannot read HttpOnly cookies, so Client
 * Components ask for the active branch's sellers through this route instead of hitting the
 * Go API directly. The branch comes from the caller (the manual RFQ form knows the one it is
 * filling in); the API still validates it against the caller's reach and answers 403 on an
 * inaccessible one, so a client-named branch widens nothing. When none is named the session
 * cookie decides, and with neither the seller list is account-wide.
 */
export async function GET(request: Request) {
  const jar = await cookies();
  const token = jar.get(ACCESS_COOKIE)?.value;
  if (!token) {
    return NextResponse.json({ code: 'UNAUTHENTICATED' }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const branchId = searchParams.get('branch_id') ?? jar.get(BRANCH_COOKIE)?.value;

  const headers = new Headers();
  headers.set('Authorization', `Bearer ${token}`);
  if (branchId) {
    headers.set('X-Branch-Id', branchId);
  }

  const upstream = await fetch(`${API_URL}/v1/sellers`, {
    method: 'GET',
    headers,
    cache: 'no-store',
  });

  const text = await upstream.text();
  return new NextResponse(text, {
    status: upstream.status,
    headers: { 'Content-Type': 'application/json' },
  });
}
