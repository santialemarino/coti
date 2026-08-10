'use server';

import { revalidatePath } from 'next/cache';

import { accountSchema, type AccountValues } from '@/app/(protected)/settings/account/form-schema';
import { ROUTES } from '@/config/routes';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';

export interface AccountResult {
  ok?: true;
  error?: ApiErrorCode;
}

export async function updateAccount(values: AccountValues): Promise<AccountResult> {
  // Re-validated server-side: the client's schema is a courtesy, not a guarantee.
  const parsed = accountSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

  // Built before the try, so a mapping bug here surfaces as itself instead of as the same
  // 'INTERNAL' the API path answers with.
  const body = bodyOf(parsed.data);
  try {
    await apiRequest({ path: '/v1/account', method: 'PUT', body });
  } catch (error) {
    return { error: errorCodeOf(error) };
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
