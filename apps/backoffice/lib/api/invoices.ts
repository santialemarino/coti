import 'server-only';

import { apiRequest } from '@/lib/api/client';
import { mapInvoice, type InvoiceRaw } from '@/lib/api/invoicing';

export async function getInvoices(branchId: string, page: number) {
  const result = await apiRequest<{
    items: InvoiceRaw[];
    total: number;
    page: number;
    page_size: number;
  }>({ path: '/v1/invoices', branchId, query: { page: String(page) } });
  return {
    items: result.items.map(mapInvoice),
    total: result.total,
    page: result.page,
    pageSize: result.page_size,
  };
}
