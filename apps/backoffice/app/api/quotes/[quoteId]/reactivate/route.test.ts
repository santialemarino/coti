import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { cookieJar } from '@repo/vitest-config/cookies';
import { ACCESS_COOKIE } from '@/lib/auth/tokens';

vi.mock('next/headers', () => ({ cookies: vi.fn() }));

const { cookies } = await import('next/headers');
const { POST } = await import('@/app/api/quotes/[quoteId]/reactivate/route');

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

describe('the quote reactivation proxy', () => {
  it('forwards the selected mode and the quote branch', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(Response.json({}, { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    const body = JSON.stringify({ mode: 'EDIT' });

    const response = await POST(
      new Request('http://localhost/api/quotes/reactivate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-Branch-Id': BRANCH_ID },
        body,
      }),
      { params: Promise.resolve({ quoteId: QUOTE_ID }) },
    );

    expect(response.status).toBe(200);
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(url).toContain(`/v1/quotes/${QUOTE_ID}/reactivate`);
    expect(init?.method).toBe('POST');
    expect(init?.body).toBe(body);
    expect(new Headers(init?.headers).get('X-Branch-Id')).toBe(BRANCH_ID);
  });
});
