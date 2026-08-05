import 'server-only';

import { cache } from 'react';

import { apiRequest } from '@/lib/api/client';

// --- Raw types (API JSON shape, snake_case) ---

interface AccountRaw {
  id: string;
  name: string;
  legal_name: string | null;
  tax_id: string | null;
  brand_logo_url: string | null;
  brand_color: string | null;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

// --- Frontend types (camelCase) ---

export interface Account {
  id: string;
  name: string;
  legalName: string | null;
  taxId: string | null;
  brandLogoUrl: string | null;
  brandColor: string | null;
}

// --- Mappers ---

function mapAccount(raw: AccountRaw): Account {
  return {
    id: raw.id,
    name: raw.name,
    legalName: raw.legal_name,
    taxId: raw.tax_id,
    brandLogoUrl: raw.brand_logo_url,
    brandColor: raw.brand_color,
  };
}

// --- API functions ---

/*
 * The corralón's own record. Reading it is not admin-only on the API — anything naming the account
 * needs it — while writing it is. `is_active` is not carried: an inactive account cannot hold a
 * session at all, so every caller who gets this far belongs to an active one.
 */
export const getAccount = cache(async (): Promise<Account> => {
  return mapAccount(await apiRequest<AccountRaw>({ path: '/v1/account' }));
});
