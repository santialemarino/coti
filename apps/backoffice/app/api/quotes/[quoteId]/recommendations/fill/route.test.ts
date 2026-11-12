import { beforeEach, describe, expect, it, vi } from 'vitest';

const fetchMock = vi.fn();
vi.mock('next/headers', () => ({
  cookies: vi.fn(async () => ({ get: vi.fn(() => ({ value: 'access-token' })) })),
}));

const { POST } = await import('@/app/api/quotes/[quoteId]/recommendations/fill/route');

describe('recommendation fill proxy', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal(
      'fetch',
      fetchMock.mockResolvedValue(new Response('{"items":[]}', { status: 200 })),
    );
  });

  it('forwards the authenticated request in the selected branch', async () => {
    const response = await POST(
      new Request('http://localhost/api/quotes/q/recommendations/fill', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-Branch-Id': 'branch' },
        body: '{}',
      }),
      { params: Promise.resolve({ quoteId: 'quote' }) },
    );
    expect(response.status).toBe(200);
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(url).toContain('/v1/quotes/quote/recommendations/fill');
    expect(init?.method).toBe('POST');
    expect(new Headers(init?.headers).get('X-Branch-Id')).toBe('branch');
  });
});
