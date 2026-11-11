import 'server-only';

import { cache } from 'react';
import type { Metadata } from 'next';
import { notFound } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { customerApiPath } from '@/lib/auth/customer-session';
import { API_URL } from '@/lib/config';

export interface CustomerAuthOptions {
  googleClientId: string;
  passwordMinLength: number;
  supplierName: string;
  logoUrl?: string;
}

// The supplier portal's public identity, read once per request by every page that shows it.
export const getCustomerAuthOptions = cache(async (): Promise<CustomerAuthOptions> => {
  const response = await fetch(await customerApiPath('/auth/google/options'), {
    cache: 'no-store',
  });
  if (response.status === 404) notFound();
  if (!response.ok) throw new Error('Customer authentication configuration unavailable');
  const options = (await response.json()) as {
    client_id: string;
    password_min_length: number;
    supplier_name: string;
    logo_url?: string;
  };
  return {
    googleClientId: options.client_id,
    passwordMinLength: options.password_min_length,
    supplierName: options.supplier_name,
    logoUrl: options.logo_url?.startsWith('/') ? API_URL + options.logo_url : options.logo_url,
  };
});

// A tab on the customer portal is named after the corralón the customer is dealing with.
export async function customerPageMetadata(
  section: 'title' | 'profile' | 'newRequest',
): Promise<Metadata> {
  const [options, t] = await Promise.all([
    getCustomerAuthOptions(),
    getTranslations('customerOrders'),
  ]);
  return { title: `${t(section)} · ${options.supplierName}` };
}
