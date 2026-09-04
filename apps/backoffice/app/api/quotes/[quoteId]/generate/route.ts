import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';

import { ACCESS_COOKIE, BRANCH_COOKIE } from '@/lib/auth/tokens';
import { API_URL } from '@/lib/config';

/*
 * BFF proxy for POST /v1/quotes/:quoteId/accept-materials. Prices a draft quote
 * and moves it to QUOTED.
 */
export async function POST(
  _request: Request,
  { params }: { params: Promise<{ quoteId: string }> },
) {
  const { quoteId } = await params;
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

  const upstream = await fetch(`${API_URL}/v1/quotes/${quoteId}/accept-materials`, {
    method: 'POST',
    headers,
    cache: 'no-store',
  });

  const text = await upstream.text();
  return new NextResponse(text, {
    status: upstream.status,
    headers: { 'Content-Type': 'application/json' },
  });
}
