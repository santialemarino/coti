import 'server-only';

import { cache } from 'react';

import { apiRequest } from '@/lib/api/client';
import type { LowerPriceStrategy } from '@/lib/constants/conversation';

// --- Raw types (API JSON shape, snake_case) ---

interface AccountRaw {
  id: string;
  name: string;
  legal_name: string | null;
  tax_id: string | null;
  brand_logo_url: string | null;
  brand_color: string | null;
  preferred_lower_price_strategy: LowerPriceStrategy;
  invoicing_enabled: boolean;
  has_invoices: boolean;
  invoice_on_payment: boolean;
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
  preferredLowerPriceStrategy: LowerPriceStrategy;
  // Whether the corralón invoices with Coti; off hides everything invoicing-related.
  invoicingEnabled: boolean;
  hasInvoices: boolean;
  // Whether «Marcar cobrado» starts with «Facturar también» ticked.
  invoiceOnPayment: boolean;
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
    preferredLowerPriceStrategy: raw.preferred_lower_price_strategy,
    invoicingEnabled: raw.invoicing_enabled,
    hasInvoices: raw.has_invoices,
    invoiceOnPayment: raw.invoice_on_payment,
  };
}

// Issued invoices are fiscal records, so their history stays reachable after invoicing is turned off.
export function showsInvoiceHistory(account: Pick<Account, 'invoicingEnabled' | 'hasInvoices'>) {
  return account.invoicingEnabled || account.hasInvoices;
}

// --- API functions ---

/*
 * The corralón's own record. Reading it is not admin-only on the API — anything naming the account
 * needs it — while writing it is. `is_active` is not carried: an inactive account cannot hold a
 * session at all, so every caller who gets this far belongs to an active one.
 */
export const getAccount = cache(async (): Promise<Account> => {
  return mapAccount(await apiRequest<AccountRaw>({ path: '/v1/account', branchScoped: false }));
});
