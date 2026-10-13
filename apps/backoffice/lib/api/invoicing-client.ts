import { throwApiError } from '@/lib/api/errors';
import {
  mapInvoice,
  mapInvoicePreview,
  type Invoice,
  type InvoicePreview,
  type InvoicePreviewRaw,
  type InvoiceRaw,
} from '@/lib/api/invoicing';

// What an accepted order would be invoiced with now, every gap that stops it, and its invoice.
export async function getInvoicePreview(
  quoteId: string,
  branchId: string,
): Promise<InvoicePreview> {
  const response = await fetch(`/api/quotes/${encodeURIComponent(quoteId)}/invoice`, {
    headers: { 'X-Branch-Id': branchId },
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
  return mapInvoicePreview((await response.json()) as InvoicePreviewRaw);
}

// Issues exactly the previewed invoice; the API refuses with INVOICE_STALE if the sale moved since.
export async function issueInvoice(
  quoteId: string,
  branchId: string,
  preview: Pick<InvoicePreview, 'fingerprint' | 'versionId' | 'type' | 'amounts'>,
): Promise<Invoice> {
  const response = await fetch(`/api/quotes/${encodeURIComponent(quoteId)}/invoice`, {
    method: 'POST',
    headers: { 'X-Branch-Id': branchId, 'Content-Type': 'application/json' },
    body: JSON.stringify({
      fingerprint: preview.fingerprint,
      version_id: preview.versionId,
      type: preview.type,
      total: preview.amounts.total,
    }),
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
  return mapInvoice((await response.json()) as InvoiceRaw);
}

// Asks ARCA how a pending invoice ended, without requesting another authorization.
export async function recoverInvoice(invoiceId: string, branchId: string): Promise<Invoice> {
  const response = await fetch(`/api/invoices/${encodeURIComponent(invoiceId)}/recover`, {
    method: 'POST',
    headers: { 'X-Branch-Id': branchId },
    cache: 'no-store',
  });
  if (!response.ok) await throwApiError(response);
  return mapInvoice((await response.json()) as InvoiceRaw);
}
