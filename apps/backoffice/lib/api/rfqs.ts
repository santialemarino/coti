/*
 * The statuses a seller sees and reasons about. DRAFT is deliberately absent: it is an internal
 * quote_state while the RFQ is still GENERATED, never a visible business state (see
 * docs/internal/domain/estados.md). The wire may still carry it, so every binding point shoves it
 * through normalizeRfqStatus.
 */
export type RfqStatus =
  | 'RECEIVED'
  | 'FAILED'
  | 'GENERATED'
  | 'QUOTED'
  | 'SENT'
  | 'CHANGE_REQUESTED'
  | 'ACCEPTED'
  | 'REJECTED';

/*
 * The backend merges quote.current_status into the RFQ status it hands back, so a raw response can
 * say DRAFT while the seller sees GENERATED. Normalize at the display boundary: DRAFT collapses onto
 * GENERATED, everything else passes through untouched.
 */
export function normalizeRfqStatus(status: string): RfqStatus {
  const upper = status.toUpperCase();
  if (upper === 'DRAFT' || upper === 'GENERATED') return 'GENERATED';
  return upper as RfqStatus;
}

// The channel a request arrived through; an icon and an i18n label hang off each value.
export type RfqChannel = 'whatsapp' | 'email' | 'webapp' | 'manual_entry';

export interface RfqRecord {
  id: string;
  quoteNumber: number | null;
  client: string;
  createdAt: string;
  channel: RfqChannel;
  seller: string;
  // The resolved seller user id; null when the quote has none assigned yet.
  sellerId: string | null;
  branch: string;
  branchId: string;
  itemCount: number;
  /*
   * Decimal string, the wire format money travels as (NUMERIC(14,2)); currency is per-account.
   * Absent until the quote exists — an uncotized request has no amount to show.
   */
  total?: string;
  status: RfqStatus;
  // Backend-set flag: the seller must chase this quote; it surfaces first and is highlighted.
  needsFollowup: boolean;
  processing?: boolean;
  archived?: boolean;
}

// Raw shape returned by GET /v1/rfqs — the mapper lives on the server side.
export interface RfqListItem {
  id: string;
  quote_number: number | null;
  client: string | null;
  created_at: string;
  channel: string;
  seller_id: string | null;
  seller: string;
  branch: string;
  branch_id: string;
  item_count: number;
  total: string | null;
  status: string;
  needs_followup: boolean;
  archived_at: string | null;
}

const RFQ_REFERENCE_MIN_DIGITS = 2;

export function formatRfqReference(quoteNumber: number | null | undefined): string | null {
  if (quoteNumber == null) return null;
  return `#${String(quoteNumber).padStart(RFQ_REFERENCE_MIN_DIGITS, '0')}`;
}

// Raw shape returned by GET /v1/rfqs/:rfqId — the detail view projection.
export interface RfqDetailResponse {
  rfq: RfqListItem;
  quote: QuoteResponse | null;
  version: QuoteVersionResponse | null;
  items: QuoteItemResponse[];
  alternatives: Record<string, QuoteItemAlternativeResponse[]>;
  rfq_status_history: RfqStatusChangeResponse[];
  quote_status_history: QuoteStatusChangeResponse[];
  deliveries: QuoteSendTrackingResponse[];
  discounts?: QuoteDiscountResponse[];
  changes_requested?: ChangeRequestDiff;
}

export interface QuoteResponse {
  id: string;
  branch_id: string;
  client_id: string | null;
  rfq_id: string;
  seller_id: string | null;
  current_version_id: string | null;
  current_status: string;
  expires_at: string | null;
  archived_at: string | null;
  needs_followup: boolean;
  followup_flagged_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface QuoteVersionResponse {
  id: string;
  quote_id: string;
  author_id: string | null;
  version_number: number;
  total: string;
  is_immutable: boolean;
  comment: string | null;
  created_at: string;
}

export interface QuoteItemResponse {
  id: string;
  version_id: string;
  product_id: string | null;
  product_code: string | null;
  product_name: string | null;
  product_unit: string | null;
  requested_description: string;
  quantity: string;
  unit: string | null;
  unit_price_snapshot: string | null;
  min_price_snapshot: string | null;
  subtotal: string | null;
  confidence_score: string | null;
  match_status: string;
  alternatives: QuoteItemAlternativeResponse[];
  pricing_unavailable: boolean | null;
  quantity_rationale: string | null;
  created_at: string;
}

export interface QuoteItemAlternativeResponse {
  id: string;
  product_id: string | null;
  combo_id: string | null;
  type: string;
  origin: string;
  rank: number;
  confidence_score: string | null;
  price_snapshot: string | null;
  approved_by_seller: boolean;
  chosen_by_client: boolean;
  code: string | null;
  canonical_name: string | null;
  unit: string | null;
}

export type DiscountScope = 'TOTAL' | 'ITEM' | 'ITEM_SET';
export type DiscountActionType = 'FIXED_AMOUNT' | 'PERCENTAGE';

export interface RfqStatusChangeResponse {
  id: string;
  rfq_id: string;
  previous_status: string | null;
  new_status: string;
  user_id: string | null;
  changed_at: string;
  created_at: string;
}

export interface QuoteStatusChangeResponse {
  id: string;
  quote_id: string;
  previous_status: string | null;
  new_status: string;
  user_id: string | null;
  changed_at: string;
  created_at: string;
}

export interface QuoteSendTrackingResponse {
  id: string;
  version_id: string;
  channel: string;
  destination: string;
  format: string;
  tracking_status: string;
  sent_at: string | null;
  expires_at: string | null;
  created_at: string;
}

/*
 * Returned by POST /v1/rfqs/file-drafts. Quote and version are absent when the model read no
 * material out of the file: the RFQ and the file are kept and the seller works it by hand.
 */
export interface FileRfqDraftResponse {
  rfq: { id: string; status: string };
  quote: QuoteResponse | null;
  version: QuoteVersionResponse | null;
  items: QuoteItemResponse[];
}

// Body of POST /v1/quotes/:quoteId/sends. WhatsApp and the public link always go; the email
// copy is the optional one, so an absent email_delivery means "WhatsApp only".
export interface QuoteSendBody {
  recipient_phone: string;
  email_delivery?: { address: string } | null;
  expiry_days?: number | null;
}

export interface QuoteSendResponse {
  quote_id: string;
  version_id: string;
  current_status: string;
  expires_at: string | null;
  deliveries: QuoteDeliveryResponse[];
}

export interface QuoteDeliveryResponse {
  id: string;
  channel: string;
  destination: string;
  tracking_status: string;
  public_url: string;
  sent_at: string | null;
}

export type DiscountOrigin = 'AUTOMATIC' | 'AI_ADAPTATION' | 'MANUAL_SELLER';

export interface QuoteDiscountResponse {
  id: string;
  quote_version_id: string;
  promotion_id: string | null;
  promotion_name: string | null;
  condition_type: string;
  scope: DiscountScope;
  origin: DiscountOrigin;
  amount: string;
  action_type: DiscountActionType;
  // The raw rule the seller typed, null for engine-applied discounts; decimals are strings.
  action_value: string | null;
  // Lines an ITEM/ITEM_SET discount covers; empty for TOTAL and on post/patch responses.
  item_ids: string[];
  description: string | null;
  suppressed_by_seller: boolean;
  created_at: string;
}

/*
 * Create/update body for a seller-typed discount, matching the backend DTOs. The backend computes
 * the money amount from the rule (fixed ≤ scope base, percentage of the scope base) and recomputes
 * the version total; item_ids are required for ITEM/ITEM_SET scopes.
 */
export interface CreateDiscountBody {
  description: string;
  action_type: DiscountActionType;
  value: string;
  scope: DiscountScope;
  item_ids?: string[];
}

export interface UpdateDiscountBody {
  description?: string;
  action_type?: DiscountActionType;
  value?: string;
  scope?: DiscountScope;
  item_ids?: string[];
  suppressed_by_seller?: boolean;
}

export interface DiffLineItem {
  description: string;
  quantity: string;
  unit: string | null;
  unit_price: string | null;
  changed: boolean;
  change_type?: 'modified' | 'added' | 'removed';
}

export interface DiffDiscountLine {
  name: string;
  amount: string;
  changed: boolean;
}

export interface ChangeRequestDiff {
  reason: string | null;
  original: {
    items: DiffLineItem[];
    discounts: DiffDiscountLine[];
    total: string;
  };
  requested: {
    items: DiffLineItem[];
    discounts: DiffDiscountLine[];
    total: string;
  };
}
