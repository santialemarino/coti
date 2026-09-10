import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { cookieJar } from '@repo/vitest-config/cookies';
import { ACCESS_COOKIE } from '@/lib/auth/tokens';

vi.mock('next/headers', () => ({ cookies: vi.fn() }));

const { cookies } = await import('next/headers');
const { POST } = await import('@/app/api/quotes/[quoteId]/generate/route');

const BRANCH_ID = '11111111-1111-4111-8111-111111111111';
const QUOTE_ID = '22222222-2222-4222-8222-222222222222';

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(cookies).mockResolvedValue(
    cookieJar({ [ACCESS_COOKIE]: 'access-token' }) as unknown as Awaited<
      ReturnType<typeof cookies>
    >,
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('the quote generation proxy', () => {
  it('uses the only reachable branch when none was explicitly selected', async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(Response.json({ items: [{ id: BRANCH_ID }] }, { status: 200 }))
      .mockResolvedValueOnce(Response.json({ quote: {}, version: {}, items: [] }, { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    const response = await POST(new Request('http://localhost/api/quotes/generate'), {
      params: Promise.resolve({ quoteId: QUOTE_ID }),
    });

    expect(response.status).toBe(200);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    const [url, init] = fetchMock.mock.calls[1] ?? [];
    expect(url).toContain(`/v1/quotes/${QUOTE_ID}/accept-materials`);
    expect(new Headers(init?.headers).get('X-Branch-Id')).toBe(BRANCH_ID);
  });
});
