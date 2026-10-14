import 'server-only';

import { apiRequest } from '@/lib/api/client';
import { mapInvoice, type Invoice, type InvoiceRaw } from '@/lib/api/invoicing';

interface InvoicePageRaw {
  items: InvoiceRaw[];
  total: number;
  page: number;
  page_size: number;
}

export interface InvoicePage {
  items: Invoice[];
  total: number;
  page: number;
  pageSize: number;
}

// One page of the fiscal history, for the active branch or every branch the caller reaches.
export async function getInvoices(page: number): Promise<InvoicePage> {
  const result = await apiRequest<InvoicePageRaw>({
    path: '/v1/invoices',
    query: { page: String(page) },
  });
  return {
    items: result.items.map(mapInvoice),
    total: result.total,
    page: result.page,
    pageSize: result.page_size,
  };
}
