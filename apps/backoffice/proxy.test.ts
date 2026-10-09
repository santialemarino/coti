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
  // The root is the public landing for everyone; it never sends a visitor to log in.
  it('lets a visitor through to the landing at the root', async () => {
    const response = await proxy(visitor('/'));

    expect(response.headers.get('location')).toBeNull();
    expect(response.headers.get('x-middleware-rewrite')).toBeNull();
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

  // A seller lands on the queue, and a lapsed one is renewed there rather than treated as a visitor.
  it('keeps the queue behind the gate and renews a lapsed session on it', async () => {
    vi.mocked(requestRefresh).mockResolvedValue({
      ok: false,
      status: 401,
      code: 'UNAUTHENTICATED',
    });
    const response = await proxy(
      new NextRequest('https://backoffice.test/inbox', {
        headers: { cookie: `${REFRESH_COOKIE}=refresh` },
      }),
    );

    expect(requestRefresh).toHaveBeenCalled();
    expect(new URL(response.headers.get('location') ?? '').pathname).toBe('/login');
  });
});

const TOKENS = { accessToken: 'fresh-access', refreshToken: 'fresh-refresh' };

function withRefresh(path: string) {
  return new NextRequest(`https://backoffice.test${path}`, {
    headers: { cookie: `${REFRESH_COOKIE}=refresh` },
  });
}

describe('proxy settling a lapsed session', () => {
  // The access token lapses within minutes; the refresh token is what says someone is signed in.
  it('renews a seller on the login screen and sends them where `next` asked', async () => {
    vi.mocked(requestRefresh).mockResolvedValue({ ok: true, status: 200, tokens: TOKENS });
    const response = await proxy(withRefresh('/login?next=%2Frfqs%2Fa1'));

    expect(new URL(response.headers.get('location') ?? '').pathname).toBe('/rfqs/a1');
    expect(response.headers.get('set-cookie')).toContain('fresh-access');
  });

  it('shows the login screen once a revoked session has been cleared', async () => {
    vi.mocked(requestRefresh).mockResolvedValue({
      ok: false,
      status: 401,
      code: 'UNAUTHENTICATED',
    });
    const response = await proxy(withRefresh('/login'));

    expect(response.headers.get('location')).toBeNull();
    expect(response.headers.get('set-cookie')).toContain(`${REFRESH_COOKIE}=;`);
  });

  // The landing reads the cookies after this, so they have to mean a live session.
  it('renews on the public site too, without sending anyone anywhere', async () => {
    vi.mocked(requestRefresh).mockResolvedValue({ ok: true, status: 200, tokens: TOKENS });
    const response = await proxy(withRefresh('/'));

    expect(response.headers.get('location')).toBeNull();
    expect(response.headers.get('set-cookie')).toContain('fresh-access');
  });

  // The page renders from the request's cookies, so they have to be gone there too, not only in the reply.
  it('renders the public page for a visitor once a revoked session is cleared', async () => {
    vi.mocked(requestRefresh).mockResolvedValue({
      ok: false,
      status: 401,
      code: 'UNAUTHENTICATED',
    });
    const response = await proxy(withRefresh('/'));

    expect(response.headers.get('x-middleware-request-cookie') ?? '').not.toContain('refresh');
    expect(response.headers.get('set-cookie')).toContain(`${REFRESH_COOKIE}=;`);
  });

  // A deploy's 503 is not a revoked session; ending it would log a seller out for nothing.
  it('keeps the session on a public page when the renewal only hiccups', async () => {
    vi.mocked(requestRefresh).mockResolvedValue({ ok: false, status: 503, code: 'INTERNAL' });
    const response = await proxy(withRefresh('/'));

    expect(response.headers.get('set-cookie')).toBeNull();
    expect(response.headers.get('location')).toBeNull();
  });

  it('ends a lapsed session that has nothing to renew it with', async () => {
    const response = await proxy(
      new NextRequest('https://backoffice.test/', {
        headers: { cookie: `${ACCESS_COOKIE}=expired` },
      }),
    );

    expect(response.headers.get('set-cookie')).toContain(`${ACCESS_COOKIE}=;`);
    expect(response.headers.get('location')).toBeNull();
  });

  it('never spends a refresh token on a prefetch', async () => {
    const response = await proxy(
      new NextRequest('https://backoffice.test/', {
        headers: { cookie: `${REFRESH_COOKIE}=refresh`, 'next-router-prefetch': '1' },
      }),
    );

    expect(requestRefresh).not.toHaveBeenCalled();
    expect(response.headers.get('location')).toBeNull();
  });

  it('lets a visitor see the login screen', async () => {
    const response = await proxy(visitor('/login'));

    expect(response.headers.get('location')).toBeNull();
    expect(requestRefresh).not.toHaveBeenCalled();
  });

  it('never renews on session-ended, whose job is clearing the cookies', async () => {
    const response = await proxy(withRefresh('/session-ended'));

    expect(requestRefresh).not.toHaveBeenCalled();
    expect(response.headers.get('location')).toBeNull();
  });
});
