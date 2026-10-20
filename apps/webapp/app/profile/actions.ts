'use server';

import { getTranslations } from 'next-intl/server';

import { customerProfileSchema, type CustomerProfileValues } from '@/app/profile/form-schema';
import { ROUTES } from '@/config/routes';
import { customerFetch } from '@/lib/api/customer-orders';
import { requireCustomer } from '@/lib/auth/customer-flow';

export async function saveCustomerProfile(
  values: CustomerProfileValues,
): Promise<{ error?: string }> {
  await requireCustomer(ROUTES.profile);
  const t = await getTranslations('customerProfile.errors');
  const parsed = customerProfileSchema(t).safeParse(values);
  if (!parsed.success) return { error: t('invalid') };
  try {
    const response = await customerFetch('/profile', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        ...parsed.data,
        contacts: parsed.data.contacts.filter((contact) => contact.value !== ''),
      }),
    });
    if (!response.ok)
      return {
        error: t(
          response.status === 409
            ? 'conflict'
            : response.status === 400 || response.status === 422
              ? 'invalid'
              : 'unavailable',
        ),
      };
    return {};
  } catch {
    return { error: t('unavailable') };
  }
}
