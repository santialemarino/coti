import { NextRequest } from 'next/server';
import { describe, expect, it, vi } from 'vitest';

import { POST } from '@/app/api/public-rfqs/[channelId]/route';

const params = Promise.resolve({ channelId: '12345678-1234-4234-8234-123456789012' });

describe('public RFQ forwarding boundary', () => {
  it('forwards the body without session credentials and returns a minimal receipt', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ received: true, account_id: 'hidden' }), { status: 201 }),
      );
    vi.stubGlobal('fetch', fetchMock);
    const request = new NextRequest('http://localhost/api/public-rfqs/channel', {
      method: 'POST',
      body: JSON.stringify({ name: 'Obra Norte' }),
      headers: {
        'Content-Type': 'application/json',
        Cookie: 'session=private',
        Authorization: 'Bearer private',
      },
    });
    const response = await POST(request, { params });
    expect(await response.json()).toEqual({ received: true });
    expect(response.headers.get('set-cookie')).toBeNull();
    expect(response.headers.get('cache-control')).toBe('no-store');
    const forwarded = new Headers(fetchMock.mock.calls[0]![1].headers);
    expect(forwarded.get('authorization')).toBeNull();
    expect(forwarded.get('cookie')).toBeNull();
  });

  it('preserves a rate-limit refusal without exposing the API prose', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ code: 'RATE_LIMITED', error: 'internal detail' }), {
          status: 429,
        }),
      ),
    );
    const request = new NextRequest('http://localhost/api/public-rfqs/channel', {
      method: 'POST',
      body: '{}',
    });
    const response = await POST(request, { params });
    expect(response.status).toBe(429);
    expect(await response.json()).toEqual({ code: 'RATE_LIMITED' });
  });
});
