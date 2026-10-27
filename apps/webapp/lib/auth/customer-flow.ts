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
export const CUSTOMER_COOKIE_OPTIONS = {
  httpOnly: true,
  secure: process.env.NODE_ENV === 'production',
  sameSite: 'lax',
  path: '/',
} as const;

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
