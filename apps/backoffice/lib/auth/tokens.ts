/*
 * Session primitives that must run in the edge runtime as well as on the server,
 * because middleware.ts is the one place a session is renewed. Nothing here may
 * import next/headers or server-only.
 *
 * The access token is opaque to the backoffice. It is forwarded, never inspected
 * for authorization: verifying it would mean holding the API's HMAC signing key,
 * and a symmetric key in a second service is a key that can mint tokens for any
 * account. Who the caller is comes from GET /v1/me; whether they may do something
 * is the API's answer on every request.
 */

import {
  API_URL,
  REFRESH_SKEW_SECONDS,
  REMEMBERED_SESSION_MAX_AGE_SECONDS,
  TRUSTED_PROXY_HOPS,
} from '@/lib/config';

export const ACCESS_COOKIE = 'coti_access_token';
export const REFRESH_COOKIE = 'coti_refresh_token';
// Marks the session as remembered, so a renewal in middleware keeps it that way.
export const REMEMBER_COOKIE = 'coti_remember';
// The branch every request is scoped to. Named here rather than beside its reader so the
// gate, which cannot import server-only, can clear it alongside the session.
export const BRANCH_COOKIE = 'coti_branch';

// httpOnly keeps the token out of reach of client code. lax still sends it on a
// top-level navigation, which is what following a link from a mail client is.
const BASE_COOKIE_OPTIONS = {
  httpOnly: true,
  sameSite: 'lax',
  secure: process.env.NODE_ENV === 'production',
  path: '/',
} as const;

/*
 * A remembered session persists; a plain one dies with the browser. The ceiling
 * matches the API's own remember-me window — the API is still what decides whether a
 * refresh token is live, so an over-long cookie buys nothing but a wasted round trip.
 */
export function sessionCookieOptions(rememberMe: boolean) {
  return rememberMe
    ? { ...BASE_COOKIE_OPTIONS, maxAge: REMEMBERED_SESSION_MAX_AGE_SECONDS }
    : BASE_COOKIE_OPTIONS;
}

export interface TokenPair {
  accessToken: string;
  refreshToken: string;
}

export interface AuthAttempt {
  ok: boolean;
  status: number;
  tokens?: TokenPair;
}

/*
 * forwardedClientAddress works out the browser's address so the API can count a rate limit
 * against it. Without it every user's unauthenticated request carries this server's address
 * and they share one allowance.
 *
 * The hop is counted from the end whatever sits in front of this app appends to, for the same
 * reason the API does it: anything the browser wrote itself lands to the left. Undefined when
 * nothing is in front, which is the local case.
 */
export function forwardedClientAddress(headers: Headers): string | undefined {
  if (TRUSTED_PROXY_HOPS <= 0) return undefined;

  const forwarded = headers.get('x-forwarded-for');
  if (!forwarded) return undefined;

  const hops = forwarded
    .split(',')
    .map((entry) => entry.trim())
    .filter(Boolean);
  const index = hops.length - TRUSTED_PROXY_HOPS;
  if (index < 0 || index >= hops.length) return undefined;
  return hops[index];
}

// The exp claim is read without verifying it, which is safe because the worst a
// forged one buys is an unnecessary refresh.
export function needsRenewal(token: string | undefined): boolean {
  if (!token) return true;
  const expiresAt = readExpiry(token);
  if (expiresAt === null) return true;
  return expiresAt - Date.now() / 1000 <= REFRESH_SKEW_SECONDS;
}

export async function requestLogin(
  email: string,
  password: string,
  rememberMe: boolean,
  callerAddress?: string,
): Promise<AuthAttempt> {
  return postForTokens(
    '/v1/public/auth/login',
    { email, password, remember_me: rememberMe },
    callerAddress,
  );
}

export async function requestRefresh(
  refreshToken: string,
  callerAddress?: string,
): Promise<AuthAttempt> {
  return postForTokens('/v1/public/auth/refresh', { refresh_token: refreshToken }, callerAddress);
}

// Reports failure rather than throwing: the cookies come off either way.
export async function requestLogout(tokens: Partial<TokenPair>): Promise<boolean> {
  if (!tokens.accessToken) return false;
  try {
    const response = await fetch(`${API_URL}/v1/auth/logout`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${tokens.accessToken}`,
      },
      body: JSON.stringify({ refresh_token: tokens.refreshToken ?? '' }),
      cache: 'no-store',
    });
    return response.ok;
  } catch {
    return false;
  }
}

async function postForTokens(
  path: string,
  body: Record<string, string | boolean>,
  callerAddress?: string,
): Promise<AuthAttempt> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  // Login and refresh are the routes with no bearer to count by, so the browser's address is
  // the only thing that keeps one user's allowance off another's.
  if (callerAddress) headers['X-Forwarded-For'] = callerAddress;

  let response: Response;
  try {
    response = await fetch(`${API_URL}${path}`, {
      method: 'POST',
      headers,
      body: JSON.stringify(body),
      cache: 'no-store',
    });
  } catch {
    // An unreachable API is not a rejected credential, and must not read as one.
    return { ok: false, status: 0 };
  }
  if (!response.ok) return { ok: false, status: response.status };

  const payload = (await response.json()) as { access_token?: string; refresh_token?: string };
  if (!payload.access_token || !payload.refresh_token) return { ok: false, status: 502 };
  return {
    ok: true,
    status: response.status,
    tokens: { accessToken: payload.access_token, refreshToken: payload.refresh_token },
  };
}

function readExpiry(token: string): number | null {
  const segment = token.split('.')[1];
  if (!segment) return null;
  try {
    const json = atob(segment.replace(/-/g, '+').replace(/_/g, '/'));
    const exp = (JSON.parse(json) as { exp?: unknown }).exp;
    return typeof exp === 'number' ? exp : null;
  } catch {
    return null;
  }
}
