import 'server-only';

import { cookies, headers } from 'next/headers';
import { redirect } from 'next/navigation';

import { ROUTES } from '@/config/routes';
import { forwardedClientAddress } from '@/lib/api/forwarded-address';
import { customerApiPath, getCustomerSession, getCustomerToken } from '@/lib/auth/customer-session';

export const GOOGLE_STATE_COOKIE =
  process.env.NODE_ENV === 'production' ? '__Host-coti_google_state' : 'coti_google_state';
export const GOOGLE_SIGNUP_COOKIE =
  process.env.NODE_ENV === 'production' ? '__Host-coti_google_signup' : 'coti_google_signup';
export const GOOGLE_EMAIL_COOKIE =
  process.env.NODE_ENV === 'production' ? '__Host-coti_google_email' : 'coti_google_email';
export const GOOGLE_EMAIL_PROVEN_COOKIE =
  process.env.NODE_ENV === 'production'
    ? '__Host-coti_google_email_proven'
    : 'coti_google_email_proven';
export const CUSTOMER_COOKIE_OPTIONS = {
  httpOnly: true,
  secure: process.env.NODE_ENV === 'production',
  sameSite: 'lax',
  path: '/',
} as const;

/* An expired Secure set: a bare delete carries no Secure flag, so a browser drops it for a __Host- cookie. */
export function clearCustomerCookie(jar: Awaited<ReturnType<typeof cookies>>, name: string) {
  jar.set(name, '', { ...CUSTOMER_COOKIE_OPTIONS, expires: new Date(0) });
}

export function safeCustomerDestination(value: unknown, fallback: string = ROUTES.orders): string {
  if (
    typeof value !== 'string' ||
    value.includes('\\') ||
    [...value].some((character) => character.charCodeAt(0) < 32)
  )
    return fallback;
  if (
    value === ROUTES.orders ||
    value === ROUTES.profile ||
    value === ROUTES.newRequest ||
    /^\/verify-contact(?:\?[^#]*)?$/.test(value) ||
    /^\/google-complete(?:\?[^#]*)?$/.test(value) ||
    /^\/(orders|quotes|rfq)\/[A-Za-z0-9_-]+(?:\?[^#]*)?$/.test(value)
  )
    return value;
  return fallback;
}

export async function requireCustomer(destination: string) {
  const session = await getCustomerSession();
  if (!session)
    redirect(`${ROUTES.login}?next=${encodeURIComponent(safeCustomerDestination(destination))}`);
  return session;
}

export async function postCustomerAuth(path: string, body: unknown, authenticated = false) {
  const token = authenticated ? await getCustomerToken() : undefined;
  const address = forwardedClientAddress(await headers());
  return fetch(await customerApiPath(path), {
    method: 'POST',
    cache: 'no-store',
    headers: {
      'Content-Type': 'application/json',
      ...(address ? { 'X-Forwarded-For': address } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(body),
  });
}

export async function pendingGoogleSignup(): Promise<string | undefined> {
  return (await cookies()).get(GOOGLE_SIGNUP_COOKIE)?.value;
}

/* The address Google verified, offered as an optional contact; display only, the API keeps its own. */
export async function pendingGoogleEmail(): Promise<string | undefined> {
  return (await cookies()).get(GOOGLE_EMAIL_COOKIE)?.value || undefined;
}

/* Whether choosing that address verifies it; otherwise it joins pending a code. Display only. */
export async function pendingGoogleEmailProven(): Promise<boolean> {
  return (await cookies()).get(GOOGLE_EMAIL_PROVEN_COOKIE)?.value === '1';
}
