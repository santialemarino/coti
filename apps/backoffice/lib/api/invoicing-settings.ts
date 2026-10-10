import 'server-only';

import { apiRequest } from '@/lib/api/client';
import {
  mapInvoicingSettings,
  type InvoicingSettings,
  type InvoicingSettingsRaw,
} from '@/lib/api/invoicing';

// The account's invoicing setup, admin-only on the API. Branches come back closed ones included.
export async function getInvoicingSettings(): Promise<InvoicingSettings> {
  const raw = await apiRequest<InvoicingSettingsRaw>({
    path: '/v1/invoicing/settings',
    branchScoped: false,
  });
  return mapInvoicingSettings(raw);
}
