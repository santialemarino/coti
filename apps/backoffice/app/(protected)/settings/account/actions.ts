'use server';

import { revalidatePath } from 'next/cache';

import { accountSchema, type AccountValues } from '@/app/(protected)/settings/account/form-schema';
import { ROUTES } from '@/config/routes';
import { uploadAccountLogo } from '@/lib/api/account-logo';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';

export interface AccountResult {
  ok?: true;
  error?: ApiErrorCode;
}

export async function updateAccount(
  values: AccountValues,
  logo?: File | null,
  // Confirms that a new CUIT removes the ARCA connection made with the old one.
  disconnectARCA = false,
): Promise<AccountResult> {
  // Re-validated server-side: the client's schema is a courtesy, not a guarantee.
  const parsed = accountSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };

  try {
    const brandLogoUrl =
      logo === undefined
        ? parsed.data.brandLogoUrl
        : logo === null
          ? ''
          : await uploadAccountLogo(logo);
    const body = { ...bodyOf({ ...parsed.data, brandLogoUrl }), disconnect_arca: disconnectARCA };
    await apiRequest({ path: '/v1/account', method: 'PUT', body });
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
  revalidatePath(ROUTES.accountSettings);
  // Removing the connection changes what the invoicing screens show.
  if (disconnectARCA) revalidatePath(ROUTES.invoicingSettings, 'layout');
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
    brand_color: values.brandColor ? `#${values.brandColor}` : undefined,
    preferred_lower_price_strategy: values.lowerPriceStrategy,
  };
}
