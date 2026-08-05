'use server';

import { revalidatePath } from 'next/cache';

import { accountSchema, type AccountValues } from '@/app/(protected)/settings/account/form-schema';
import { ROUTES } from '@/config/routes';
import { apiRequest, errorCodeOf } from '@/lib/api/client';

export type AccountErrorKey = 'invalid' | 'notFound' | 'unauthorized' | 'unexpected';

export interface AccountResult {
  ok?: true;
  error?: AccountErrorKey;
}

export async function updateAccount(values: AccountValues): Promise<AccountResult> {
  // Re-validated server-side: the client's schema is a courtesy, not a guarantee.
  const parsed = accountSchema().safeParse(values);
  if (!parsed.success) return { error: 'invalid' };

  // Built before the try, so a mapping bug here surfaces as itself instead of as the same
  // 'unexpected' the API path answers with.
  const body = bodyOf(parsed.data);
  try {
    await apiRequest({ path: '/v1/account', method: 'PUT', body });
  } catch (error) {
    const code = errorCodeOf(error);
    if (code === 'notFound') return { error: 'notFound' };
    if (code === 'forbidden' || code === 'unauthenticated') return { error: 'unauthorized' };
    // A 400 or a 422 both mean this form and the API's own validation have drifted apart: the
    // blank name and both brand formats are refused here before the request is made.
    if (code === 'badRequest' || code === 'unprocessable') return { error: 'invalid' };
    return { error: 'unexpected' };
  }
  // Only this route: no other screen renders the account, so the shell has nothing to refresh.
  revalidatePath(ROUTES.accountSettings);
  return { ok: true };
}

function bodyOf(values: AccountValues) {
  return {
    name: values.name,
    /*
     * Omitted rather than empty, which is also how a value is cleared: the API's optional fields
     * are pointers with `omitempty`, and that only skips a nil one — a pointer to "" passes
     * validation and lands in the column, so "no logo" would become a logo of nothing.
     */
    legal_name: values.legalName || undefined,
    tax_id: values.taxId || undefined,
    brand_logo_url: values.brandLogoUrl || undefined,
    brand_color: values.brandColor || undefined,
  };
}
