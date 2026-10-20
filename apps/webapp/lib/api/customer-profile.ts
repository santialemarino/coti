import 'server-only';

import { customerFetch } from '@/lib/api/customer-orders';
import {
  mapCustomerProfile,
  type CustomerProfile,
  type CustomerProfileRaw,
} from '@/lib/api/customer-profile-model';
import { CustomerAuthError } from '@/lib/auth/customer-session';

export async function getCustomerProfile(): Promise<CustomerProfile> {
  const response = await customerFetch('/profile');
  if (!response.ok) throw new CustomerAuthError(response.status);
  return mapCustomerProfile((await response.json()) as CustomerProfileRaw);
}
