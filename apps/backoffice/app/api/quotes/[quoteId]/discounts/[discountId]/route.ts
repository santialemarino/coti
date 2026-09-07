import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';

import { ACCESS_COOKIE, BRANCH_COOKIE } from '@/lib/auth/tokens';
import { API_URL } from '@/lib/config';

/*
 * BFF proxy for PATCH /v1/quotes/:quoteId/discounts/:discountId. Patches a discount application
 * on a mutable version; the backend recomputes the version total when a write lands.
 */
export async function PATCH(
  request: Request,
  { params }: { params: Promise<{ quoteId: string; discountId: string }> },
) {
  const { quoteId, discountId } = await params;
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

  const upstream = await fetch(`${API_URL}/v1/quotes/${quoteId}/discounts/${discountId}`, {
    method: 'PATCH',
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

/*
 * BFF proxy for DELETE /v1/quotes/:quoteId/discounts/:discountId. Removes a MANUAL_SELLER
 * discount for good; the backend refuses deleting an engine-applied one.
 */
export async function DELETE(
  _request: Request,
  { params }: { params: Promise<{ quoteId: string; discountId: string }> },
) {
  const { quoteId, discountId } = await params;
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

  const upstream = await fetch(`${API_URL}/v1/quotes/${quoteId}/discounts/${discountId}`, {
    method: 'DELETE',
    headers,
    cache: 'no-store',
  });

  const text = await upstream.text();
  return new NextResponse(text, {
    status: upstream.status,
    headers: { 'Content-Type': 'application/json' },
  });
}
