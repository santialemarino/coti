import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('next/headers', () => ({ headers: vi.fn() }));

const { headers } = await import('next/headers');
const { siteOrigin } = await import('@/lib/utils/site-origin');

function withHeaders(values: Record<string, string>) {
  vi.mocked(headers).mockResolvedValue(new Headers(values) as never);
}

beforeEach(() => vi.clearAllMocks());

describe('siteOrigin', () => {
  // Behind the platform's edge the Host header names the container; the forwarded one names the site.
  it('prefers the forwarded host and protocol over the direct ones', async () => {
    withHeaders({
      host: 'backoffice:3000',
      'x-forwarded-host': 'coti.ar',
      'x-forwarded-proto': 'https',
    });

    expect(await siteOrigin()).toBe('https://coti.ar');
  });

  it('takes the first hop when a proxy chain lists several', async () => {
    withHeaders({
      'x-forwarded-host': 'coti.ar, edge.internal',
      'x-forwarded-proto': 'https, http',
    });

    expect(await siteOrigin()).toBe('https://coti.ar');
  });

  it('falls back to the Host header over plain http when nothing forwarded the request', async () => {
    withHeaders({ host: 'localhost:3000' });

    expect(await siteOrigin()).toBe('http://localhost:3000');
  });
});
