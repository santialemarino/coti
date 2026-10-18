import { NextRequest, NextResponse } from 'next/server';

import { forwardedClientAddress } from '@/lib/api/forwarded-address';
import { API_URL } from '@/lib/config';

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ channelId: string }> },
) {
  const { channelId } = await params;
  const headers = { 'Cache-Control': 'no-store' };
  if (!/^[0-9a-f-]{36}$/i.test(channelId)) {
    return NextResponse.json({ code: 'NOT_FOUND' }, { status: 404, headers });
  }
  try {
    const upstreamHeaders = new Headers({
      'Content-Type': request.headers.get('content-type') ?? 'application/json',
    });
    const address = forwardedClientAddress(request.headers);
    if (address) upstreamHeaders.set('X-Forwarded-For', address);
    // Streaming keeps file size enforcement at the API boundary without buffering it in Next.
    const upstream = await fetch(`${API_URL}/v1/public/rfq-channels/${channelId}/requests`, {
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
