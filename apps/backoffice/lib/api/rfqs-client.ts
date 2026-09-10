import { ApiError, codeForStatus, knownErrorCode } from '@/lib/api/errors';
import type {
  CreateDiscountBody,
  FileRfqDraftResponse,
  QuoteDiscountResponse,
  QuoteItemResponse,
  QuoteResponse,
  QuoteSendBody,
  QuoteSendResponse,
  QuoteVersionResponse,
  RfqDetailResponse,
  UpdateDiscountBody,
} from '@/lib/api/rfqs';

interface CreateRfqBody {
  client_label?: string | null;
  work_type?: string | null;
  seller_id?: string | null;
  items: {
    product_id?: string | null;
    requested_description: string;
    quantity: string;
    unit?: string | null;
  }[];
}

export interface CreateRfqResponse {
  rfq: {
    id: string;
    branch_id: string;
    client_label: string | null;
    channel_id: string;
    raw_text: string | null;
    status: string;
    work_type: string | null;
    received_at: string;
    created_at: string;
  };
  quote: {
    id: string;
    branch_id: string;
    rfq_id: string;
    seller_id: string | null;
    current_version_id: string | null;
    current_status: string;
    created_at: string;
    updated_at: string;
  };
}

/*
 * Client-side RFQ creation. Posts to the internal BFF route that reads the
 * session and branch cookies server-side and forwards them as auth headers.
 * This module has no server-only imports and is safe to use from Client Components.
 */
export async function createRfq(body: CreateRfqBody): Promise<CreateRfqResponse> {
  const response = await fetch('/api/rfqs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    cache: 'no-store',
  });

  if (!response.ok) {
    let code: string | undefined;
    let detail = '';
    try {
      const payload = (await response.json()) as {
        error?: string;
        code?: string;
        detail?: string;
      };
      code = payload.code;
      detail = [payload.error, payload.detail].filter(Boolean).join(': ');
    } catch {
      // A body that is not the envelope tells us nothing extra.
    }
    throw new ApiError(
      knownErrorCode(code) ?? codeForStatus(response.status),
      response.status,
      detail || undefined,
    );
  }

  return response.json() as Promise<CreateRfqResponse>;
}

export interface FileRfqDraftFields {
  channelId: string;
  clientLabel?: string;
  workType?: string;
  note?: string;
}

/*
 * Run an order that arrived as a file through the AI pipeline. The API reads the file — a
 * photo, a PDF, a spreadsheet or a voice note — stores it against the RFQ, and answers with
 * the same seller-reviewable draft the text flow produces.
 */
export async function createFileRfqDraft(
  file: File,
  fields: FileRfqDraftFields,
  branchId: string | null,
): Promise<FileRfqDraftResponse> {
  const body = new FormData();
  body.set('file', file);
  body.set('channel_id', fields.channelId);
  if (fields.clientLabel) body.set('client_label', fields.clientLabel);
  if (fields.workType) body.set('work_type', fields.workType);
  if (fields.note) body.set('note', fields.note);

  const response = await fetch('/api/rfqs/file-drafts', {
    method: 'POST',
    headers: branchId ? { 'X-Branch-Id': branchId } : undefined,
    body,
    cache: 'no-store',
  });

  if (!response.ok) {
    await throwOnError(response);
  }
  return response.json() as Promise<FileRfqDraftResponse>;
}

/*
 * Client-side RFQ detail fetch. Calls the internal BFF route that reads the
 * session and branch cookies server-side and forwards them as auth headers.
 */
export async function fetchRfqDetail(id: string): Promise<RfqDetailResponse> {
  const response = await fetch(`/api/rfqs/${id}`, {
    method: 'GET',
    cache: 'no-store',
  });

  if (!response.ok) {
    let code: string | undefined;
    let detail = '';
    try {
      const payload = (await response.json()) as {
        error?: string;
        code?: string;
        detail?: string;
      };
      code = payload.code;
      detail = [payload.error, payload.detail].filter(Boolean).join(': ');
    } catch {
      // A body that is not the envelope tells us nothing extra.
    }
    throw new ApiError(
      knownErrorCode(code) ?? codeForStatus(response.status),
      response.status,
      detail || undefined,
    );
  }

  return response.json() as Promise<RfqDetailResponse>;
}

/*
 * The branch a write is scoped to. An order belongs to exactly one branch, and the header
 * switcher is a filter over the list — it sits on "todas las sucursales" by default — so a
 * screen acting on one order names that order's branch instead of the current filter.
 */
function branchHeaders(branchId: string, json = false): Record<string, string> {
  return {
    'X-Branch-Id': branchId,
    ...(json ? { 'Content-Type': 'application/json' } : {}),
  };
}

/*
 * Helper to throw an ApiError from a non-ok response. Extracts the code and
 * detail from the standard error envelope when present.
 */
async function throwOnError(response: Response): Promise<never> {
  let code: string | undefined;
  let detail = '';
  try {
    const payload = (await response.json()) as {
      error?: string;
      code?: string;
      detail?: string;
    };
    code = payload.code;
    detail = [payload.error, payload.detail].filter(Boolean).join(': ');
  } catch {
    // A body that is not the envelope tells us nothing extra.
  }
  throw new ApiError(
    knownErrorCode(code) ?? codeForStatus(response.status),
    response.status,
    detail || undefined,
  );
}

/*
 * Claim an unassigned order for the signed-in seller. The backend answers 409
 * when a peer took it first; the caller surfaces that as a conflict toast.
 */
export async function assignRfqSeller(rfqId: string): Promise<QuoteResponse> {
  const response = await fetch(`/api/rfqs/${rfqId}/assign`, {
    method: 'POST',
    cache: 'no-store',
  });

  if (!response.ok) {
    await throwOnError(response);
  }
  return response.json() as Promise<QuoteResponse>;
}

/*
 * Admin steering of an order's owner: seller_id names the seller to put on the order, or null
 * clears the assignment. The backend is admin-only and refuses a seller who does not serve the
 * order's own branch.
 */
export async function setRfqSeller(rfqId: string, sellerId: string | null): Promise<QuoteResponse> {
  const response = await fetch(`/api/rfqs/${rfqId}/seller`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ seller_id: sellerId }),
    cache: 'no-store',
  });

  if (!response.ok) {
    await throwOnError(response);
  }
  return response.json() as Promise<QuoteResponse>;
}

/*
 * Patch an editable quote item. Only provided fields are written.
 */
export async function updateQuoteItem(
  quoteId: string,
  itemId: string,
  branchId: string,
  body: {
    product_id?: string | null;
    requested_description?: string;
    quantity?: string;
    unit?: string | null;
    unit_price_snapshot?: string;
  },
): Promise<QuoteItemResponse> {
  const response = await fetch(`/api/quotes/${quoteId}/items/${itemId}`, {
    method: 'PATCH',
    headers: branchHeaders(branchId, true),
    body: JSON.stringify(body),
    cache: 'no-store',
  });

  if (!response.ok) {
    await throwOnError(response);
  }
  return response.json() as Promise<QuoteItemResponse>;
}

/*
 * Delete a draft quote item.
 */
export async function deleteQuoteItem(
  quoteId: string,
  itemId: string,
  branchId: string,
): Promise<void> {
  const response = await fetch(`/api/quotes/${quoteId}/items/${itemId}`, {
    method: 'DELETE',
    headers: branchHeaders(branchId),
    cache: 'no-store',
  });

  if (!response.ok) {
    await throwOnError(response);
  }
}

/*
 * Add a new item to a draft quote version.
 */
export async function addQuoteItem(
  quoteId: string,
  branchId: string,
  body: {
    product_id?: string | null;
    requested_description: string;
    quantity: string;
    unit?: string | null;
  },
): Promise<QuoteItemResponse> {
  const response = await fetch(`/api/quotes/${quoteId}/items`, {
    method: 'POST',
    headers: branchHeaders(branchId, true),
    body: JSON.stringify(body),
    cache: 'no-store',
  });

  if (!response.ok) {
    await throwOnError(response);
  }
  return response.json() as Promise<QuoteItemResponse>;
}

/*
 * Generate (price) a draft quote. Calls the existing accept-materials endpoint.
 */
export async function generateQuote(
  quoteId: string,
  branchId: string,
): Promise<{ quote: QuoteResponse; version: QuoteVersionResponse; items: QuoteItemResponse[] }> {
  const response = await fetch(`/api/quotes/${quoteId}/generate`, {
    method: 'POST',
    headers: branchHeaders(branchId),
    cache: 'no-store',
  });

  if (!response.ok) {
    await throwOnError(response);
  }
  return response.json();
}

/*
 * Add a seller-typed discount. The backend computes the amount from the rule (action_type + value +
 * scope + item_ids) and recomputes the version total.
 */
export async function addDiscount(
  quoteId: string,
  branchId: string,
  body: CreateDiscountBody,
): Promise<QuoteDiscountResponse> {
  const response = await fetch(`/api/quotes/${quoteId}/discounts`, {
    method: 'POST',
    headers: branchHeaders(branchId, true),
    body: JSON.stringify(body),
    cache: 'no-store',
  });

  if (!response.ok) {
    await throwOnError(response);
  }
  return response.json() as Promise<QuoteDiscountResponse>;
}

/*
 * Patch a discount application. Only present fields are written; a suppression toggle leaves
 * the amount alone.
 */
export async function updateDiscount(
  quoteId: string,
  discountId: string,
  branchId: string,
  body: UpdateDiscountBody,
): Promise<QuoteDiscountResponse> {
  const response = await fetch(`/api/quotes/${quoteId}/discounts/${discountId}`, {
    method: 'PATCH',
    headers: branchHeaders(branchId, true),
    body: JSON.stringify(body),
    cache: 'no-store',
  });

  if (!response.ok) {
    await throwOnError(response);
  }
  return response.json() as Promise<QuoteDiscountResponse>;
}

/*
 * Delete a MANUAL_SELLER discount for good. The backend refuses deleting an engine-applied
 * discount; those are suppressed instead.
 */
export async function deleteDiscount(
  quoteId: string,
  discountId: string,
  branchId: string,
): Promise<void> {
  const response = await fetch(`/api/quotes/${quoteId}/discounts/${discountId}`, {
    method: 'DELETE',
    headers: branchHeaders(branchId),
    cache: 'no-store',
  });

  if (!response.ok) {
    await throwOnError(response);
  }
}

/*
 * Deliver an approved quote to its client. The API freezes the version, always attempts
 * WhatsApp with the public link, and attempts email independently when an address is given —
 * each destination reports its own outcome, so a failed one does not hide a delivered one.
 */
export async function sendQuote(
  quoteId: string,
  branchId: string,
  body: QuoteSendBody,
): Promise<QuoteSendResponse> {
  const response = await fetch(`/api/quotes/${quoteId}/sends`, {
    method: 'POST',
    headers: { ...branchHeaders(branchId, true), 'Idempotency-Key': crypto.randomUUID() },
    body: JSON.stringify(body),
    cache: 'no-store',
  });

  if (!response.ok) {
    await throwOnError(response);
  }
  return response.json() as Promise<QuoteSendResponse>;
}
