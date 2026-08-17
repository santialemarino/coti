import { beforeEach, describe, expect, it, vi } from 'vitest';

import { cookieJar } from '@repo/vitest-config/cookies';
import { ROUTES } from '@/config/routes';
import { ApiError } from '@/lib/api/errors';
import { clearSession, getAccessToken, getSession, requireAdmin } from '@/lib/auth/session';
import { ACCESS_COOKIE, BRANCH_COOKIE, REFRESH_COOKIE, REMEMBER_COOKIE } from '@/lib/auth/tokens';

vi.mock('next/headers', () => ({ cookies: vi.fn() }));
vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error('NEXT_NOT_FOUND');
  }),
  redirect: vi.fn((path: string) => {
    throw new Error(`NEXT_REDIRECT:${path}`);
  }),
}));
// Fully, not partially: the real client pulls in the branch reader, which pulls the branch
// list back through the client, and a session unit test has no business in that cycle. The error
// vocabulary is left real — it is a pure module, and mocking it would hide the mapping.
vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn() }));

const { cookies } = await import('next/headers');
const { notFound, redirect } = await import('next/navigation');
const { apiRequest } = await import('@/lib/api/client');

const ADMIN = { id: 'u1', account_id: 'a1', name: 'Ana', email: 'ana@coti.test', role: 'ADMIN' };
const SELLER = { ...ADMIN, id: 'u2', name: 'Beto', role: 'SELLER' };

function jar(initial: Record<string, string> = { [ACCESS_COOKIE]: 'token' }) {
  const fake = cookieJar(initial);
  vi.mocked(cookies).mockResolvedValue(fake as unknown as Awaited<ReturnType<typeof cookies>>);
  return fake;
}

beforeEach(() => vi.clearAllMocks());

describe('getSession', () => {
  /*
   * Identity does not depend on a branch, and a stale branch cookie answering 403 here would
   * read as a dead session and sign the caller out of the app instead of failing one screen.
   */
  it('asks for the caller without naming a branch', async () => {
    jar();
    vi.mocked(apiRequest).mockResolvedValue(ADMIN);

    await getSession();
    expect(apiRequest).toHaveBeenCalledWith({ path: '/v1/me', branchScoped: false });
  });
});

describe('requireAdmin', () => {
  it('hands the session back to an admin', async () => {
    jar();
    vi.mocked(apiRequest).mockResolvedValue(ADMIN);

    await expect(requireAdmin()).resolves.toMatchObject({ userId: 'u1', role: 'ADMIN' });
    expect(notFound).not.toHaveBeenCalled();
  });

  /*
   * 404 and not 403: a seller who guesses an admin URL should not learn the page is there.
   * The API refuses them anyway — this only decides what the refusal looks like.
   */
  it('answers a seller with a 404 rather than admitting the page exists', async () => {
    jar();
    vi.mocked(apiRequest).mockResolvedValue(SELLER);

    await expect(requireAdmin()).rejects.toThrow('NEXT_NOT_FOUND');
    expect(redirect).not.toHaveBeenCalled();
  });

  // A page renders alongside the layout that guards it, so a session that died in between
  // must land on the same screen the layout would have sent them to.
  it('sends a caller with no session to the session-ended screen', async () => {
    jar({});

    await expect(requireAdmin()).rejects.toThrow(`NEXT_REDIRECT:${ROUTES.sessionEnded}`);
    expect(notFound).not.toHaveBeenCalled();
  });

  it('sends a caller the API no longer knows to the same screen', async () => {
    jar();
    vi.mocked(apiRequest).mockRejectedValue(new ApiError('UNAUTHENTICATED', 401));

    await expect(requireAdmin()).rejects.toThrow(`NEXT_REDIRECT:${ROUTES.sessionEnded}`);
  });
});

describe('getAccessToken', () => {
  /*
   * The same trap the branch cookie hit: a delete leaves the entry blank for the rest of the
   * request. Every consumer today happens to test truthiness, so `''` behaves like absence by
   * luck — one `??` would end that, and the reader is where the invariant belongs.
   */
  it('reports no token once the session has been cleared in this request', async () => {
    jar();
    await clearSession();

    await expect(getAccessToken()).resolves.toBeUndefined();
  });
});

describe('clearSession', () => {
  // The branch is this user's choice, not the browser's: leaving it behind would start the
  // next person to sign in on this machine inside a branch they never picked.
  it('drops the branch alongside the session cookies', async () => {
    const cookieJar = jar();

    await clearSession();
    [ACCESS_COOKIE, REFRESH_COOKIE, REMEMBER_COOKIE, BRANCH_COOKIE].forEach((name) => {
      expect(cookieJar.delete).toHaveBeenCalledWith(name);
    });
  });
});
