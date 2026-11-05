'use server';

import { revalidatePath } from 'next/cache';

import { ROUTES } from '@/config/routes';
import { apiRequest } from '@/lib/api/client';
import { getCustomerPortal } from '@/lib/api/customer-portal';
import { errorCodeOf } from '@/lib/api/errors';

export async function loadPortal() {
  try {
    return { data: await getCustomerPortal() };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

export async function checkPortal(subdomain: string) {
  try {
    const data = await apiRequest<{ available: boolean }>({
      path: '/v1/account/portal/availability',
      query: { subdomain },
      branchScoped: false,
    });
    return { available: data.available };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}

export async function savePortal(
  subdomain: string,
  expectedHostname: string,
  confirmChange: boolean,
) {
  try {
    await apiRequest({
      path: '/v1/account/portal',
      method: 'PUT',
      branchScoped: false,
      body: { subdomain, expected_hostname: expectedHostname, confirm_change: confirmChange },
    });
    revalidatePath(ROUTES.onboarding);
    revalidatePath(ROUTES.accountSettings);
    revalidatePath(ROUTES.home, 'layout');
    return { ok: true as const };
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
}
