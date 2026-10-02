import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';

import { ACCESS_COOKIE } from '@/lib/auth/tokens';
import { API_URL } from '@/lib/config';

/*
 * BFF proxy for GET /v1/products. The catalog is account-scoped, so no
 * X-Branch-Id header is forwarded — the API returns every product the
 * account owns regardless of the active branch.
 */
export async function GET(request: Request) {
  const jar = await cookies();
  const token = jar.get(ACCESS_COOKIE)?.value;
  if (!token) {
    return NextResponse.json({ code: 'UNAUTHENTICATED' }, { status: 401 });
  }

  const url = new URL(request.url);
  const upstreamUrl = new URL(`${API_URL}/v1/products`);
  const search = url.searchParams.get('search');
  const limit = url.searchParams.get('limit');
  const offset = url.searchParams.get('offset');
  if (search) upstreamUrl.searchParams.set('search', search);
  if (limit) upstreamUrl.searchParams.set('limit', limit);
  if (offset) upstreamUrl.searchParams.set('offset', offset);

  const upstream = await fetch(upstreamUrl.toString(), {
    method: 'GET',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    cache: 'no-store',
  });

  const text = await upstream.text();
  return new NextResponse(text, {
    status: upstream.status,
    headers: { 'Content-Type': 'application/json' },
  });
}
