'use server';

import { cookies, headers } from 'next/headers';
import { redirect } from 'next/navigation';

import { ROUTES } from '@/config/routes';
import { forwardedClientAddress } from '@/lib/api/forwarded-address';
import {
  GOOGLE_SIGNUP_COOKIE,
  pendingGoogleSignup,
  postCustomerAuth,
  safeCustomerDestination,
} from '@/lib/auth/customer-flow';
import {
  acceptCustomerSession,
  CustomerAuthError,
  endCustomerSession,
  startCustomerSession,
  type CustomerLoginRaw,
} from '@/lib/auth/customer-session';

export interface CustomerAuthResult {
  error?: string;
}

function errorForStatus(status: number): string {
  return status === 409
    ? 'conflict'
    : status === 429
      ? 'locked'
      : status === 401
        ? 'credentials'
        : status === 400 || status === 422
          ? 'invalid'
          : 'unavailable';
}

export async function signInCustomer(values: {
  taxId: string;
  password: string;
  next: string;
}): Promise<CustomerAuthResult> {
  try {
    await startCustomerSession(
      'login',
      values.taxId,
      values.password,
      forwardedClientAddress(await headers()),
    );
  } catch (error) {
    return {
      error: error instanceof CustomerAuthError ? errorForStatus(error.status) : 'unavailable',
    };
  }
  redirect(safeCustomerDestination(values.next));
}

export async function signUpCustomer(values: {
  name: string;
  phone: string;
  taxId: string;
  password: string;
  email: string;
  next: string;
}): Promise<CustomerAuthResult> {
  try {
    await startCustomerSession(
      'register',
      values.taxId,
      values.password,
      forwardedClientAddress(await headers()),
      { name: values.name, phone: values.phone, email: values.email },
    );
  } catch (error) {
    return {
      error: error instanceof CustomerAuthError ? errorForStatus(error.status) : 'unavailable',
    };
  }
  redirect(safeCustomerDestination(values.next, ROUTES.newRequest));
}

export async function completeCustomerGoogle(values: {
  name?: string;
  taxId: string;
  next: string;
  link?: boolean;
}): Promise<CustomerAuthResult> {
  const token = await pendingGoogleSignup();
  if (!token) return { error: 'expired' };
  try {
    const response = await postCustomerAuth(
      values.link ? '/auth/google/link' : '/auth/google/complete',
      { token, tax_id: values.taxId, name: values.name },
      values.link,
    );
    if (!response.ok) return { error: errorForStatus(response.status) };
    await acceptCustomerSession((await response.json()) as CustomerLoginRaw);
    (await cookies()).delete(GOOGLE_SIGNUP_COOKIE);
  } catch {
    return { error: 'unavailable' };
  }
  redirect(safeCustomerDestination(values.next, ROUTES.newRequest));
}

export async function signOutCustomer(): Promise<void> {
  await endCustomerSession();
  redirect(ROUTES.login);
}
