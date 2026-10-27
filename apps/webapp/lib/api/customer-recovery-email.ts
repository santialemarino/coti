import 'server-only';

import { customerFetch } from '@/lib/api/customer-orders';
import { CustomerAuthError } from '@/lib/auth/customer-session';

export interface CustomerRecoveryEmail {
  email: string | null;
  verified: boolean;
}

export async function getCustomerRecoveryEmail(): Promise<CustomerRecoveryEmail> {
  const response = await customerFetch('/auth/recovery-email');
  if (!response.ok) throw new CustomerAuthError(response.status);
  return response.json() as Promise<CustomerRecoveryEmail>;
}
