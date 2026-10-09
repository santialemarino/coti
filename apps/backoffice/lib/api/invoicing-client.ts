import { ApiError, codeForStatus, knownErrorCode } from '@/lib/api/errors';
import {
  mapClientFiscal,
  mapInvoice,
  mapInvoicePreview,
  type ClientFiscal,
  type ClientFiscalRaw,
  type Invoice,
  type InvoicePreview,
  type InvoicePreviewRaw,
  type InvoiceRaw,
  type IvaCondition,
} from '@/lib/api/invoicing';

export interface ClientFiscalBody {
  legal_name?: string;
  tax_id?: string;
  iva_condition?: IvaCondition;
}

// What an accepted order would be invoiced with now, every gap that stops it, and its invoice.
export async function getInvoicePreview(
  quoteId: string,
  branchId: string,
): Promise<InvoicePreview> {
  const response = await fetch(`/api/quotes/${encodeURIComponent(quoteId)}/invoice`, {
    headers: { 'X-Branch-Id': branchId },
    cache: 'no-store',
  });
  if (!response.ok) await throwOnError(response);
  return mapInvoicePreview((await response.json()) as InvoicePreviewRaw);
}

// Issues exactly the previewed invoice; the API refuses with INVOICE_STALE if the sale moved since.
export async function issueInvoice(
  quoteId: string,
  branchId: string,
  preview: Pick<InvoicePreview, 'versionId' | 'type' | 'amounts'>,
): Promise<Invoice> {
  const response = await fetch(`/api/quotes/${encodeURIComponent(quoteId)}/invoice`, {
    method: 'POST',
    headers: { 'X-Branch-Id': branchId, 'Content-Type': 'application/json' },
    body: JSON.stringify({
      version_id: preview.versionId,
      type: preview.type,
      total: preview.amounts.total,
    }),
    cache: 'no-store',
  });
  if (!response.ok) await throwOnError(response);
  return mapInvoice((await response.json()) as InvoiceRaw);
}

export async function getClientFiscal(clientId: string): Promise<ClientFiscal> {
  const response = await fetch(`/api/clients/${encodeURIComponent(clientId)}/fiscal`, {
    cache: 'no-store',
  });
  if (!response.ok) await throwOnError(response);
  return mapClientFiscal((await response.json()) as ClientFiscalRaw);
}

export async function updateClientFiscal(
  clientId: string,
  body: ClientFiscalBody,
): Promise<ClientFiscal> {
  const response = await fetch(`/api/clients/${encodeURIComponent(clientId)}/fiscal`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    cache: 'no-store',
  });
  if (!response.ok) await throwOnError(response);
  return mapClientFiscal((await response.json()) as ClientFiscalRaw);
}

// Keeps the envelope's issues, which is where ARCA's own reasons for a refusal travel.
async function throwOnError(response: Response): Promise<never> {
  let code: string | undefined;
  let detail = '';
  let issues: string[] = [];
  try {
    const payload = (await response.json()) as {
      error?: string;
      code?: string;
      issues?: string[] | null;
    };
    code = payload.code;
    detail = payload.error ?? '';
    issues = payload.issues ?? [];
  } catch {
    // A non-envelope body adds no information beyond the status.
  }
  throw new ApiError(
    knownErrorCode(code) ?? codeForStatus(response.status),
    response.status,
    detail || undefined,
    issues,
  );
}
