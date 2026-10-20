import { NextRequest, NextResponse } from 'next/server';

import { forwardedClientAddress } from '@/lib/api/forwarded-address';
import { customerApiPath, getCustomerToken } from '@/lib/auth/customer-session';
import { isSameOrigin } from '@/lib/auth/same-origin';

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ channelId: string }> },
) {
  if (!isSameOrigin(request)) return NextResponse.json({ code: 'FORBIDDEN' }, { status: 403 });
  const token = await getCustomerToken();
  if (!token) return NextResponse.json({ code: 'UNAUTHENTICATED' }, { status: 401 });
  const { channelId } = await params;
  const headers = { 'Cache-Control': 'no-store' };
  if (!/^[0-9a-f-]{36}$/i.test(channelId)) {
    return NextResponse.json({ code: 'NOT_FOUND' }, { status: 404, headers });
  }
  try {
    const upstreamHeaders = new Headers({
      'Content-Type': request.headers.get('content-type') ?? 'application/json',
      Authorization: `Bearer ${token}`,
    });
    const address = forwardedClientAddress(request.headers);
    if (address) upstreamHeaders.set('X-Forwarded-For', address);
    // Streaming keeps file size enforcement at the API boundary without buffering it in Next.
    const upstream = await fetch(await customerApiPath(`/rfq-channels/${channelId}/requests`), {
      method: 'POST',
      headers: upstreamHeaders,
      body: request.body,
      duplex: 'half',
      cache: 'no-store',
    } as RequestInit & { duplex: 'half' });
    const result = (await upstream.json()) as { received?: boolean; code?: string };
    if (!upstream.ok) {
      return NextResponse.json(
        { code: result.code ?? 'INTERNAL' },
        { status: upstream.status, headers },
      );
    }
    if (result.received !== true) throw new Error('Missing intake receipt');
    return NextResponse.json({ received: true }, { status: 201, headers });
  } catch {
    return NextResponse.json({ code: 'UNREACHABLE' }, { status: 502, headers });
  }
}
