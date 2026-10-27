import { cookies } from 'next/headers';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ACCESS_COOKIE } from '@/lib/auth/tokens';
import { forwardToApi } from './upstream';

vi.mock('next/headers', () => ({ cookies: vi.fn() }));
vi.mock('@/lib/config', () => ({ API_URL: 'http://api.internal' }));
beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn());
  vi.mocked(cookies).mockResolvedValue({
    get: (name: string) => (name === ACCESS_COOKIE ? { value: 'session-token' } : undefined),
  } as Awaited<ReturnType<typeof cookies>>);
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});
describe('forwardToApi fiscal documents', () => {
  it('forwards PDF bytes with private caching and the pinned branch', async () => {
    const bytes = new Uint8Array([37, 80, 68, 70, 45, 0, 255]);
    vi.mocked(fetch).mockResolvedValue(
      new Response(bytes, {
        headers: {
          'Content-Type': 'application/pdf',
          'Content-Disposition': 'attachment; filename="invoice.pdf"',
        },
      }),
    );
    const response = await forwardToApi(
      new Request('http://localhost/api/invoices/id/pdf', {
        headers: { 'X-Branch-Id': 'branch-id' },
      }),
      { path: '/v1/invoices/id/pdf', method: 'GET' },
    );
    expect(new Uint8Array(await response.arrayBuffer())).toEqual(bytes);
    expect(response.headers.get('Cache-Control')).toBe('private, no-store');
    expect(response.headers.get('Content-Disposition')).toContain('invoice.pdf');
    const headers = new Headers(vi.mocked(fetch).mock.calls[0]?.[1]?.headers);
    expect(headers.get('Authorization')).toBe('Bearer session-token');
    expect(headers.get('X-Branch-Id')).toBe('branch-id');
  });
  it('refuses unauthenticated document downloads before contacting the API', async () => {
    vi.mocked(cookies).mockResolvedValue({ get: () => undefined } as Awaited<
      ReturnType<typeof cookies>
    >);
    const response = await forwardToApi(null, { path: '/v1/invoices/id/pdf', method: 'GET' });
    expect(response.status).toBe(401);
    expect(fetch).not.toHaveBeenCalled();
  });
});
