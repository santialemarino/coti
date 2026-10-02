import 'server-only';

import { API_URL } from '@/lib/config';

/*
 * The public quote read. There is no session here: the unguessable token in the URL is the whole
 * access control, so this module never sends a credential and never takes one.
 *
 * The API declares no CORS and the webapp answers on its own subdomain, so this runs server-side
 * only — a browser fetch would be refused. The logo is the exception and is meant to be: an <img>
 * is not CORS-restricted, so the URL below is handed to the browser to load directly.
 */

const PUBLIC_QUOTE_PATH = '/v1/public/quote-sends';

// --- Raw types (API JSON shape, snake_case) ---

interface PublicQuoteSendRaw {
  status: string;
  expires_at: string;
  quote?: QuotePayloadRaw;
  message?: string;
  pdf_url?: string;
  customer_status?: string;
}

interface PublicQuoteActionResultRaw {
  customer_status: string;
  created_at: string;
  quote_status?: string;
}

interface QuotePayloadRaw {
  reference: string;
  version_number: number;
  approved_at: string;
  currency: string;
  supplier: SupplierRaw;
  branch: BranchRaw;
  customer: { name?: string };
  items: QuoteItemRaw[];
  discounts: QuoteDiscountRaw[];
  total: string;
  validity_note: string;
}

interface SupplierRaw {
  name: string;
  legal_name?: string;
  tax_id?: string;
  brand_color: string;
  logo_url?: string;
}

interface BranchRaw {
  name: string;
  address?: string;
}

interface QuoteItemRaw {
  requested_description: string;
  product_code?: string;
  product_name: string;
  quantity: string;
  unit?: string;
  unit_price: string;
  subtotal: string;
  alternatives: QuoteAlternativeRaw[];
}

interface QuoteAlternativeRaw {
  code?: string;
  name: string;
  unit?: string;
  unit_price: string;
}

interface QuoteDiscountRaw {
  description: string;
  amount: string;
}

// --- Frontend types (camelCase) ---

// The API answers EXPIRED with no quote body, so the payload is what separates the two states.
export type PublicQuoteStatus = 'ACTIVE' | 'EXPIRED';

// The wire value of client_action_type; this is the customer's deliberate decision on the frozen
// version, so the approval pallet mirrors the backend enum exactly.
export type CustomerActionType = 'ACCEPT' | 'REQUEST_CHANGE' | 'REJECT';

export interface PublicQuoteSend {
  status: PublicQuoteStatus;
  expiresAt: string;
  quote?: QuotePayload;
  message?: string;
  pdfUrl?: string;
  customerStatus?: CustomerActionType;
}

export interface PublicQuoteAction {
  type: CustomerActionType;
  message?: string;
}

export interface PublicQuoteActionResult {
  customerStatus: CustomerActionType;
  createdAt: string;
  quoteStatus?: string;
}

export interface QuotePayload {
  reference: string;
  versionNumber: number;
  approvedAt: string;
  currency: string;
  supplier: Supplier;
  branch: Branch;
  customerName?: string;
  items: QuoteItem[];
  discounts: QuoteDiscount[];
  total: string; // NUMERIC(14,2) decimal string, never float
  validityNote: string;
}

export interface Supplier {
  name: string;
  legalName?: string;
  taxId?: string;
  brandColor: string;
  logoUrl?: string;
}

export interface Branch {
  name: string;
  address?: string;
}

export interface QuoteItem {
  requestedDescription: string;
  productCode?: string;
  productName: string;
  quantity: string; // NUMERIC(14,2) decimal string, never float
  unit?: string;
  unitPrice: string; // NUMERIC(14,2) decimal string, never float
  subtotal: string; // NUMERIC(14,2) decimal string, never float
  alternatives: QuoteAlternative[];
}

export interface QuoteAlternative {
  code?: string;
  name: string;
  unit?: string;
  unitPrice: string; // NUMERIC(14,2) decimal string, never float
}

export interface QuoteDiscount {
  description: string;
  amount: string; // NUMERIC(14,2) decimal string, never float
}

// --- Mappers ---

function mapSupplier(raw: SupplierRaw): Supplier {
  return {
    name: raw.name,
    legalName: raw.legal_name,
    taxId: raw.tax_id,
    brandColor: raw.brand_color,
    logoUrl: absoluteLogoUrl(raw.logo_url),
  };
}

function mapItem(raw: QuoteItemRaw): QuoteItem {
  return {
    requestedDescription: raw.requested_description,
    productCode: raw.product_code,
    productName: raw.product_name,
    quantity: raw.quantity,
    unit: raw.unit,
    unitPrice: raw.unit_price,
    subtotal: raw.subtotal,
    alternatives: raw.alternatives.map(mapAlternative),
  };
}

function mapAlternative(raw: QuoteAlternativeRaw): QuoteAlternative {
  return { code: raw.code, name: raw.name, unit: raw.unit, unitPrice: raw.unit_price };
}

function mapQuotePayload(raw: QuotePayloadRaw): QuotePayload {
  return {
    reference: raw.reference,
    versionNumber: raw.version_number,
    approvedAt: raw.approved_at,
    currency: raw.currency,
    supplier: mapSupplier(raw.supplier),
    branch: { name: raw.branch.name, address: raw.branch.address },
    customerName: raw.customer.name,
    items: raw.items.map(mapItem),
    discounts: raw.discounts.map((discount) => ({
      description: discount.description,
      amount: discount.amount,
    })),
    total: raw.total,
    validityNote: raw.validity_note,
  };
}

function mapPublicQuoteSend(raw: PublicQuoteSendRaw): PublicQuoteSend {
  return {
    // Anything the wire adds later reads as expired rather than rendering a quote we cannot word.
    status: raw.status === 'ACTIVE' ? 'ACTIVE' : 'EXPIRED',
    expiresAt: raw.expires_at,
    quote: raw.quote ? mapQuotePayload(raw.quote) : undefined,
    message: raw.message,
    pdfUrl: raw.pdf_url,
    customerStatus: raw.customer_status as CustomerActionType | undefined,
  };
}

/*
 * An account that uploaded its logo stores the API's own public path, while one that pasted a URL
 * stores that URL whole. Only the first needs the API's origin in front of it, and the browser is
 * what loads either.
 */
function absoluteLogoUrl(stored: string | undefined): string | undefined {
  if (!stored) return undefined;
  return stored.startsWith('/') ? `${API_URL}${stored}` : stored;
}

// --- API functions ---

// Returns null for a token the API does not know, which the page renders as its 404.
export async function getPublicQuoteByToken(token: string): Promise<PublicQuoteSend | null> {
  const response = await fetch(`${API_URL}${PUBLIC_QUOTE_PATH}/${encodeURIComponent(token)}`, {
    method: 'GET',
    headers: { Accept: 'application/json' },
    cache: 'no-store',
  });
  if (response.status === 404) return null;
  if (!response.ok) throw new Error(`public quote read failed with ${response.status}`);
  return mapPublicQuoteSend((await response.json()) as PublicQuoteSendRaw);
}

// PublicQuoteActionError carries the HTTP status behind a refused customer response, so the app
// route can forward the right shape instead of guessing from a message string.
export class PublicQuoteActionError extends Error {
  readonly status: number;

  constructor(status: number) {
    super(`public quote action failed with ${status}`);
    this.status = status;
  }
}

/*
 * Records the customer's deliberate answer on the frozen version. Like the read above this is
 * server-only and unauthenticated: the token in the URL is the access control. The API treats a
 * repeated answer for the same send as a replay, so this is safe for the component to call twice
 * on a flaky first response.
 */
export async function submitPublicQuoteAction(
  token: string,
  action: PublicQuoteAction,
): Promise<PublicQuoteActionResult> {
  const response = await fetch(
    `${API_URL}${PUBLIC_QUOTE_PATH}/${encodeURIComponent(token)}/action`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ type: action.type, message: action.message }),
      cache: 'no-store',
    },
  );
  if (!response.ok) throw new PublicQuoteActionError(response.status);
  const raw = (await response.json()) as PublicQuoteActionResultRaw;
  return {
    customerStatus: raw.customer_status as CustomerActionType,
    createdAt: raw.created_at,
    quoteStatus: raw.quote_status,
  };
}
