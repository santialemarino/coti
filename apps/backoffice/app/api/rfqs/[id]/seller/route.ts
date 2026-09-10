import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';

import { ACCESS_COOKIE, BRANCH_COOKIE } from '@/lib/auth/tokens';
import { API_URL } from '@/lib/config';

/*
 * BFF proxy for PUT /v1/rfqs/:rfqId/seller. Admin steering: the body names the seller to put
 * on the order, or a null seller_id to clear it. The route is admin-only on the API, which
 * also checks the named seller serves the order's own branch. The branch cookie is forwarded
 * so the write stays inside the branch the operator is viewing.
 */
export async function PUT(request: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const jar = await cookies();
  const token = jar.get(ACCESS_COOKIE)?.value;
  if (!token) {
    return NextResponse.json({ code: 'UNAUTHENTICATED' }, { status: 401 });
  }

  const headers = new Headers();
  headers.set('Authorization', `Bearer ${token}`);
  headers.set('Content-Type', 'application/json');

  const branchId = jar.get(BRANCH_COOKIE)?.value;
  if (branchId) {
    headers.set('X-Branch-Id', branchId);
  }

  const body = await request.text();
  const upstream = await fetch(`${API_URL}/v1/rfqs/${id}/seller`, {
    method: 'PUT',
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
