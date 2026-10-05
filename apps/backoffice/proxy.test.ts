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
