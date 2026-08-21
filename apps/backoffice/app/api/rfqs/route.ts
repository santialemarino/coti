import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';

import { ACCESS_COOKIE, BRANCH_COOKIE } from '@/lib/auth/tokens';
import { API_URL } from '@/lib/config';

/*
 * BFF proxy for POST /v1/rfqs. The browser cannot read HttpOnly cookies, so
 * Client Components call this internal route instead of hitting the Go API
 * directly. This handler reads the session and branch cookies server-side and
 * forwards them as the Authorization and X-Branch-Id headers the API expects.
 *
 * When the branch cookie is missing (e.g. a single-branch account where the
 * switcher is hidden), the route resolves the branch by calling GET /v1/branches.
 * If there is exactly one reachable branch it is used automatically; multiple
 * branches without a selection produce a clear error.
 */
export async function POST(request: Request) {
  const jar = await cookies();
  const token = jar.get(ACCESS_COOKIE)?.value;
  if (!token) {
    return NextResponse.json({ code: 'UNAUTHENTICATED' }, { status: 401 });
  }

  const headers = new Headers();
  headers.set('Authorization', `Bearer ${token}`);
  headers.set('Content-Type', 'application/json');

  let branchId = jar.get(BRANCH_COOKIE)?.value;

  if (!branchId) {
    const branchesRes = await fetch(`${API_URL}/v1/branches`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${token}` },
      cache: 'no-store',
    });

    if (branchesRes.ok) {
      const data = (await branchesRes.json()) as { items: { id: string }[] };
      if (data.items.length === 1) {
        branchId = data.items[0]?.id ?? undefined;
      }
    }
  }

  if (branchId) {
    headers.set('X-Branch-Id', branchId);
  }

  const body = await request.text();

  const upstream = await fetch(`${API_URL}/v1/rfqs`, {
    method: 'POST',
    headers,
    body,
    cache: 'no-store',
  });

  const text = await upstream.text();
  return new NextResponse(text, {
    status: upstream.status,
    headers: { 'Content-Type': 'application/json' },
  });
}
