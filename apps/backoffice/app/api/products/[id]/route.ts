import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';

import { ACCESS_COOKIE } from '@/lib/auth/tokens';
import { API_URL } from '@/lib/config';

export async function GET(_request: Request, { params }: { params: Promise<{ id: string }> }) {
  const jar = await cookies();
  const token = jar.get(ACCESS_COOKIE)?.value;
  if (!token) return NextResponse.json({ code: 'UNAUTHENTICATED' }, { status: 401 });

  const { id } = await params;
  const response = await fetch(`${API_URL}/v1/products/${encodeURIComponent(id)}`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: 'no-store',
  });
  return new NextResponse(await response.text(), {
    status: response.status,
    headers: { 'Content-Type': 'application/json' },
  });
}
