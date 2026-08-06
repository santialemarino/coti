import 'server-only';

import { cache } from 'react';
import { cookies } from 'next/headers';
import { notFound, redirect } from 'next/navigation';

import { ROUTES } from '@/config/routes';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf } from '@/lib/api/errors';
import {
  ACCESS_COOKIE,
  BRANCH_COOKIE,
  REFRESH_COOKIE,
  REMEMBER_COOKIE,
  requestLogout,
  sessionCookieOptions,
  type TokenPair,
} from '@/lib/auth/tokens';
import { ADMIN_ROLE } from '@/lib/constants/auth';

// --- Raw types (API JSON shape, snake_case) ---

interface MeRaw {
  id: string;
  account_id: string;
  name: string;
  email: string;
  role: string;
}

// --- Frontend types (camelCase) ---

export interface SessionUser {
  userId: string;
  accountId: string;
  name: string;
  email: string;
  role: string;
}

/*
 * getSession asks the API who the caller is, so the answer accounts for what a
 * cookie cannot: a bumped session epoch, a deactivated user, a revoked token. A
 * null here means the session is over, not merely that the cookie is missing.
 *
 * Not branch-scoped: identity does not depend on a branch, and a stale cookie answering
 * 403 here would end the session rather than fail the one screen that used it. Memoised per
 * request because the shell, the section and the page each ask.
 */
export const getSession = cache(async (): Promise<SessionUser | null> => {
  if (!(await getAccessToken())) return null;
  try {
    const me = await apiRequest<MeRaw>({ path: '/v1/me', branchScoped: false });
    return {
      userId: me.id,
      accountId: me.account_id,
      name: me.name,
      email: me.email,
      role: me.role,
    };
  } catch (error) {
    const code = errorCodeOf(error);
    if (code === 'UNAUTHENTICATED' || code === 'FORBIDDEN') return null;
    throw error;
  }
});

// An admin-only page answers 404 rather than 403, so it does not advertise to a seller that
// it is there at all.
export async function requireAdmin(): Promise<SessionUser> {
  const session = await getSession();
  if (!session) redirect(ROUTES.sessionEnded);
  if (session.role !== ADMIN_ROLE) notFound();
  return session;
}

// Blank is no token: a delete leaves the entry empty for the rest of the request.
export async function getAccessToken(): Promise<string | undefined> {
  return (await cookies()).get(ACCESS_COOKIE)?.value || undefined;
}

// Next allows a cookie write only from a server action or a route handler, which is
// why the renewal path lives in middleware instead.
export async function startSession(tokens: TokenPair, rememberMe = false): Promise<void> {
  const jar = await cookies();
  const options = sessionCookieOptions(rememberMe);
  jar.set(ACCESS_COOKIE, tokens.accessToken, options);
  jar.set(REFRESH_COOKIE, tokens.refreshToken, options);
  if (rememberMe) jar.set(REMEMBER_COOKIE, '1', options);
}

// isRemembered reports what a renewal has to preserve, so a remembered session does
// not quietly decay into one that dies with the browser.
export async function isRemembered(): Promise<boolean> {
  return (await cookies()).get(REMEMBER_COOKIE)?.value === '1';
}

// clearSession drops the cookies without telling the API, for a session the API has
// already ended. The branch goes with them: it is this user's choice, not the browser's.
export async function clearSession(): Promise<void> {
  const jar = await cookies();
  jar.delete(ACCESS_COOKIE);
  jar.delete(REFRESH_COOKIE);
  jar.delete(REMEMBER_COOKIE);
  jar.delete(BRANCH_COOKIE);
}

// endSession revokes on the API and then clears. The cookies go regardless of the
// answer, so a failed revocation cannot strand a browser holding a live session.
export async function endSession(): Promise<void> {
  const jar = await cookies();
  await requestLogout({
    accessToken: jar.get(ACCESS_COOKIE)?.value,
    refreshToken: jar.get(REFRESH_COOKIE)?.value,
  });
  await clearSession();
}
