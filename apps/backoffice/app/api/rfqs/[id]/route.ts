import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';

import { ACCESS_COOKIE, BRANCH_COOKIE } from '@/lib/auth/tokens';
import { API_URL } from '@/lib/config';

/*
 * BFF proxy for GET /v1/rfqs/:rfqId. The browser cannot read HttpOnly cookies, so
 * Client Components call this internal route instead of hitting the Go API
 * directly.
 */
export async function GET(_request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const jar = await cookies();
  const token = jar.get(ACCESS_COOKIE)?.value;
  if (!token) {
    return NextResponse.json({ code: 'UNAUTHENTICATED' }, { status: 401 });
  }

  const headers = new Headers();
  headers.set('Authorization', `Bearer ${token}`);

  const branchId = jar.get(BRANCH_COOKIE)?.value;
  if (branchId) {
    headers.set('X-Branch-Id', branchId);
  }

  const upstream = await fetch(`${API_URL}/v1/rfqs/${id}`, {
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
