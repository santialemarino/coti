import 'server-only';

import { apiRequest } from '@/lib/api/client';
import { mapInvoice, type Invoice, type InvoiceRaw } from '@/lib/api/invoicing';

// --- Raw types (API JSON shape, snake_case) ---

interface PageRaw<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

interface SaleAwaitingInvoiceRaw {
  quote_id: string;
  rfq_id: string;
  quote_number: number;
  branch_id: string;
  branch_name: string;
  client_id: string | null;
  client: string;
  total: string;
  accepted_at: string;
  paid_at: string | null;
}

// --- Frontend types (camelCase) ---

export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}

export type InvoicePage = Page<Invoice>;

// An accepted sale nothing invoices, waits on ARCA for or waives yet.
export interface SaleAwaitingInvoice {
  quoteId: string;
  rfqId: string;
  quoteNumber: number;
  branchId: string;
  branchName: string;
  clientId: string | null;
  // The client's name, or the order's loose label; empty when the order names nobody.
  client: string;
  total: string;
  acceptedAt: string;
  paidAt: string | null;
}

// --- Mappers ---

function mapPage<Raw, T>(raw: PageRaw<Raw>, map: (item: Raw) => T): Page<T> {
  return { items: raw.items.map(map), total: raw.total, page: raw.page, pageSize: raw.page_size };
}

function mapSaleAwaitingInvoice(raw: SaleAwaitingInvoiceRaw): SaleAwaitingInvoice {
  return {
    quoteId: raw.quote_id,
    rfqId: raw.rfq_id,
    quoteNumber: raw.quote_number,
    branchId: raw.branch_id,
    branchName: raw.branch_name,
    clientId: raw.client_id,
    client: raw.client,
    total: raw.total,
    acceptedAt: raw.accepted_at,
    paidAt: raw.paid_at,
  };
}

// --- API functions ---

// One page of the fiscal history, for the active branch or every branch the caller reaches.
export async function getInvoices(page: number): Promise<InvoicePage> {
  const result = await apiRequest<PageRaw<InvoiceRaw>>({
    path: '/v1/invoices',
    query: { page: String(page) },
  });
  return mapPage(result, mapInvoice);
}

// One page of the sales still waiting for an invoice, paid first, in the same branch reach.
export async function getSalesAwaitingInvoice(page: number): Promise<Page<SaleAwaitingInvoice>> {
  const result = await apiRequest<PageRaw<SaleAwaitingInvoiceRaw>>({
    path: '/v1/quotes/awaiting-invoice',
    query: { page: String(page) },
  });
  return mapPage(result, mapSaleAwaitingInvoice);
}
