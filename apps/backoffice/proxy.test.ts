import { NextRequest } from 'next/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ACCESS_COOKIE, REFRESH_COOKIE } from '@/lib/auth/tokens';

vi.mock('@/lib/auth/tokens', async (importActual) => ({
  ...(await importActual<typeof import('@/lib/auth/tokens')>()),
  needsRenewal: vi.fn(() => true),
  requestRefresh: vi.fn(),
}));

const { requestRefresh } = await import('@/lib/auth/tokens');
const { proxy } = await import('./proxy');

function expiredSession(path: string) {
  return new NextRequest(`https://backoffice.test${path}`, {
    headers: { cookie: `${ACCESS_COOKIE}=expired; ${REFRESH_COOKIE}=refresh` },
  });
}

beforeEach(() => vi.clearAllMocks());

describe('proxy renewal', () => {
  // The lock is someone else guessing the password: the session ends, and the login screen says why.
  it('sends a session whose renewal is locked out to log in with the reason', async () => {
    vi.mocked(requestRefresh).mockResolvedValue({ ok: false, status: 429, code: 'ACCOUNT_LOCKED' });

    const response = await proxy(expiredSession('/rfqs'));
    const location = new URL(response.headers.get('location') ?? '');

    expect(location.pathname).toBe('/login');
    expect(location.searchParams.get('reason')).toBe('locked');
    expect(location.searchParams.get('next')).toBe('/rfqs');
    expect(response.headers.get('set-cookie')).toContain(`${ACCESS_COOKIE}=;`);
  });

  it('gives a refused renewal no reason of its own', async () => {
    vi.mocked(requestRefresh).mockResolvedValue({
      ok: false,
      status: 401,
      code: 'UNAUTHENTICATED',
    });

    const response = await proxy(expiredSession('/rfqs'));
    const location = new URL(response.headers.get('location') ?? '');

    expect(location.pathname).toBe('/login');
    expect(location.searchParams.has('reason')).toBe(false);
  });
});

function visitor(path: string) {
  return new NextRequest(`https://backoffice.test${path}`);
}

describe('proxy without a session', () => {
  it('serves the landing at the root without changing the address', async () => {
    const response = await proxy(visitor('/?utm_source=folleto'));
    const rewrite = new URL(response.headers.get('x-middleware-rewrite') ?? '');

    expect(response.headers.get('location')).toBeNull();
    expect(rewrite.pathname).toBe('/welcome');
    expect(rewrite.searchParams.get('utm_source')).toBe('folleto');
  });

  // A mistyped link must reach the 404, not a login screen promising a page that does not exist.
  it('lets an unknown path through to the 404', async () => {
    const response = await proxy(visitor('/precios'));

    expect(response.headers.get('location')).toBeNull();
    expect(response.headers.get('x-middleware-rewrite')).toBeNull();
    expect(requestRefresh).not.toHaveBeenCalled();
  });

  it('still sends a protected path to log in, remembering where it was going', async () => {
    const response = await proxy(visitor('/rfqs/a1'));
    const location = new URL(response.headers.get('location') ?? '');

    expect(location.pathname).toBe('/login');
    expect(location.searchParams.get('next')).toBe('/rfqs/a1');
  });

  // A refresh token is a seller whose access token lapsed: renew it, never show them the landing.
  it('renews a lapsed session at the root instead of serving the landing', async () => {
    vi.mocked(requestRefresh).mockResolvedValue({
      ok: false,
      status: 401,
      code: 'UNAUTHENTICATED',
    });
    const response = await proxy(
      new NextRequest('https://backoffice.test/', {
        headers: { cookie: `${REFRESH_COOKIE}=refresh` },
      }),
    );

    expect(requestRefresh).toHaveBeenCalled();
    expect(response.headers.get('x-middleware-rewrite')).toBeNull();
    expect(new URL(response.headers.get('location') ?? '').pathname).toBe('/login');
  });
});
