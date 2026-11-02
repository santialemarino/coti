import { notFound } from 'next/navigation';

import type { CustomerAuthMode } from '@/components/customer-auth-schema';
import { CustomerAuthScreen } from '@/components/customer-auth-screen';
import { safeCustomerDestination } from '@/lib/auth/customer-flow';
import { customerApiPath } from '@/lib/auth/customer-session';
import { API_URL } from '@/lib/config';

interface CustomerAuthPageProps {
  mode: CustomerAuthMode;
  next: string;
  taxId?: string;
  googleEmail?: string;
  initialError?: string;
}

export async function CustomerAuthPage(props: CustomerAuthPageProps) {
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
  return (
    <CustomerAuthScreen
      {...props}
      next={safeCustomerDestination(props.next)}
      supplierName={options.supplier_name}
      logoUrl={options.logo_url?.startsWith('/') ? API_URL + options.logo_url : options.logo_url}
      minimum={options.password_min_length}
      googleEnabled={Boolean(options.client_id)}
    />
  );
}
