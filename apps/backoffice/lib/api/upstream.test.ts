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
  it('forwards an export with its own type instead of rewriting it as JSON', async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response('\uFEFFnumero\r\n1\r\n', {
        headers: {
          'Content-Type': 'text/csv; charset=utf-8',
          'Content-Disposition': 'attachment; filename="cotizaciones-2026-11-05.csv"',
        },
      }),
    );
    const response = await forwardToApi(null, {
      path: '/v1/exports/quotes?format=csv',
      method: 'GET',
      fallbackToOnlyBranch: false,
    });
    expect(response.headers.get('Content-Type')).toBe('text/csv; charset=utf-8');
    expect(response.headers.get('Content-Disposition')).toContain('cotizaciones-2026-11-05.csv');
    expect(response.headers.get('Cache-Control')).toBe('private, no-store');
    expect(new Uint8Array(await response.arrayBuffer()).slice(0, 3)).toEqual(
      new Uint8Array([0xef, 0xbb, 0xbf]),
    );
  });
  it.each([204, 205, 304])('passes a %i through with no body', async (status) => {
    vi.mocked(fetch).mockResolvedValue(new Response(null, { status }));
    const request = new Request('http://localhost/api/clients/id/merge', {
      headers: { 'X-Branch-Id': 'branch-id' },
    });
    const response = await forwardToApi(request, {
      path: '/v1/clients/id/merge',
      method: 'POST',
      body: '{}',
    });
    expect(response.status).toBe(status);
    expect(await response.text()).toBe('');
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
