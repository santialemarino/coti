import 'server-only';

import { cache } from 'react';
import { cookies, headers } from 'next/headers';

import { API_URL, PUBLIC_SITE_HOSTNAME } from '@/lib/config';

export const CUSTOMER_SESSION_COOKIE =
  process.env.NODE_ENV === 'production' ? '__Host-coti_customer' : 'coti_customer';

interface CustomerIdentityRaw {
  id: string;
  tax_id: string;
  name: string | null;
  session_expires_at: string;
}

export interface CustomerLoginRaw {
  session_token: string;
  customer: CustomerIdentityRaw;
}

export interface CustomerSession {
  customerId: string;
  taxId: string;
  name: string | null;
  sessionExpiresAt: string;
}

export class CustomerAuthError extends Error {
  constructor(public readonly status: number) {
    super('Customer authentication failed');
    this.name = 'CustomerAuthError';
  }
}

const COOKIE_OPTIONS = {
  httpOnly: true,
  secure: process.env.NODE_ENV === 'production',
  sameSite: 'lax',
  path: '/',
} as const;

export async function getCustomerHostname(): Promise<string> {
  const host = PUBLIC_SITE_HOSTNAME || (await headers()).get('host') || '';
  return host.toLowerCase().replace(/:\d+$/, '').replace(/\.$/, '');
}

export async function getCustomerToken(): Promise<string | undefined> {
  return (await cookies()).get(CUSTOMER_SESSION_COOKIE)?.value || undefined;
}

export async function customerApiPath(path: string): Promise<string> {
  return `${API_URL}/v1/customer/sites/${encodeURIComponent(await getCustomerHostname())}${path}`;
}

export const getCustomerSession = cache(async (): Promise<CustomerSession | null> => {
  const token = await getCustomerToken();
  if (!token) return null;
  const response = await fetch(await customerApiPath('/me'), {
    cache: 'no-store',
    headers: { Authorization: `Bearer ${token}` },
  });
  if (response.status === 401) return null;
  if (!response.ok) throw new CustomerAuthError(response.status);
  return mapCustomer((await response.json()) as CustomerIdentityRaw);
});

// Session secrets remain inside the webapp server and its host-only HttpOnly cookie.
export async function startCustomerSession(
  mode: 'login' | 'register',
  taxId: string,
  password: string,
  callerAddress?: string,
  recoveryEmail?: string,
  profile?: { name: string; phone: string; email: string },
): Promise<CustomerSession> {
  const response = await fetch(await customerApiPath(`/auth/${mode}`), {
    method: 'POST',
    cache: 'no-store',
    headers: {
      'Content-Type': 'application/json',
      ...(callerAddress ? { 'X-Forwarded-For': callerAddress } : {}),
    },
    body: JSON.stringify({
      tax_id: taxId,
      password,
      ...(recoveryEmail ? { recovery_email: recoveryEmail } : {}),
      ...(profile
        ? {
            name: profile.name,
            contacts: [
              ...(profile.phone ? [{ kind: 'WHATSAPP', value: profile.phone, primary: true }] : []),
              ...(profile.email ? [{ kind: 'EMAIL', value: profile.email, primary: true }] : []),
            ],
          }
        : {}),
    }),
  });
  if (!response.ok) throw new CustomerAuthError(response.status);
  const raw = (await response.json()) as CustomerLoginRaw;
  return acceptCustomerSession(raw);
}

export async function acceptCustomerSession(raw: CustomerLoginRaw): Promise<CustomerSession> {
  const expires = new Date(raw.customer.session_expires_at);
  if (
    !/^[A-Za-z0-9_-]{43}$/.test(raw.session_token) ||
    !Number.isFinite(expires.getTime()) ||
    expires.getTime() <= Date.now()
  ) {
    throw new CustomerAuthError(502);
  }
  (await cookies()).set(CUSTOMER_SESSION_COOKIE, raw.session_token, { ...COOKIE_OPTIONS, expires });
  return mapCustomer(raw.customer);
}

export async function endCustomerSession(): Promise<void> {
  const token = await getCustomerToken();
  try {
    if (token) {
      const response = await fetch(await customerApiPath('/auth/logout'), {
        method: 'POST',
        cache: 'no-store',
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!response.ok && response.status !== 401) throw new CustomerAuthError(response.status);
    }
  } finally {
    (await cookies()).set(CUSTOMER_SESSION_COOKIE, '', { ...COOKIE_OPTIONS, expires: new Date(0) });
  }
}

function mapCustomer(raw: CustomerIdentityRaw): CustomerSession {
  return {
    customerId: raw.id,
    taxId: raw.tax_id,
    name: raw.name,
    sessionExpiresAt: raw.session_expires_at,
  };
}
