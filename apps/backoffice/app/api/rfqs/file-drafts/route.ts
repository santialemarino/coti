import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';

import { BRANCH_HEADER } from '@/lib/api/upstream';
import { ACCESS_COOKIE, BRANCH_COOKIE } from '@/lib/auth/tokens';
import { API_URL } from '@/lib/config';

/*
 * BFF proxy for POST /v1/rfqs/file-drafts. This one forwards the multipart body untouched
 * rather than going through forwardToApi, which reads the body as text: re-encoding a file
 * through a string would corrupt it and lose the part boundaries.
 */
export async function POST(request: Request) {
  const jar = await cookies();
  const token = jar.get(ACCESS_COOKIE)?.value;
  if (!token) {
    return NextResponse.json({ code: 'UNAUTHENTICATED' }, { status: 401 });
  }

  const headers = new Headers();
  headers.set('Authorization', `Bearer ${token}`);
  // The multipart Content-Type carries the boundary, so it is forwarded rather than set.
  const contentType = request.headers.get('Content-Type');
  if (contentType) {
    headers.set('Content-Type', contentType);
  }

  const branchId = request.headers.get(BRANCH_HEADER) || jar.get(BRANCH_COOKIE)?.value;
  if (branchId) {
    headers.set(BRANCH_HEADER, branchId);
  }

  const upstream = await fetch(`${API_URL}/v1/rfqs/file-drafts`, {
    method: 'POST',
    headers,
    body: await request.arrayBuffer(),
    cache: 'no-store',
  });

  const text = await upstream.text();
  return new NextResponse(text, {
    status: upstream.status,
    headers: { 'Content-Type': 'application/json' },
  });
}
